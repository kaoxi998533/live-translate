import { Ionicons } from "@expo/vector-icons";
import { Link, Redirect } from "expo-router";
import { useEffect, useRef, useState } from "react";
import { Pressable, ScrollView, StyleSheet, Text, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import {
  mediaDevices,
  registerGlobals,
  RTCSessionDescription,
  RTCPeerConnection,
  type MediaStream
} from "react-native-webrtc";
import {
  addUsage,
  createRealtimeClientSecret,
  createTranslationSession,
  endTranslationSession,
  getEntitlement,
  type Entitlement
} from "../lib/api";
import { useAuth } from "../lib/auth";
import { targetLanguages } from "../lib/languages";

registerGlobals();

const USAGE_TICK_SECONDS = 5;
const REALTIME_SDP_URL = "https://api.openai.com/v1/realtime/calls";
const quickLanguages = ["en", "zh", "ja", "ko", "es", "fr"];

type DataChannel = {
  close: () => void;
  addEventListener: (type: string, listener: (event: { data?: unknown }) => void) => void;
};

export default function TranslateScreen() {
  const { token } = useAuth();
  const [entitlement, setEntitlement] = useState<Entitlement | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [original, setOriginal] = useState("Start a session and place the phone between two speakers.");
  const [translation, setTranslation] = useState("The app will translate each speaker into the other speaker's language.");
  const [partyALanguage, setPartyALanguage] = useState("zh");
  const [partyBLanguage, setPartyBLanguage] = useState("en");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const sessionIdRef = useRef<string | null>(null);
  const peerRef = useRef<RTCPeerConnection | null>(null);
  const mediaStreamRef = useRef<MediaStream | null>(null);
  const dataChannelRef = useRef<DataChannel | null>(null);

  useEffect(() => {
    if (!token) {
      return;
    }
    refreshEntitlement(token);
  }, [token]);

  useEffect(() => {
    return () => {
      void stop();
    };
  }, []);

  if (!token) {
    return <Redirect href="/login" />;
  }

  async function refreshEntitlement(activeToken: string) {
    try {
      setEntitlement(await getEntitlement(activeToken));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not load access state");
    }
  }

  async function start() {
    if (!token) {
      return;
    }
    const activeToken = token;

    setBusy(true);
    setError(null);
    setOriginal("Connecting microphone...");
    setTranslation("Connecting to OpenAI Realtime...");

    try {
      const apiSession = await createTranslationSession(
        activeToken,
        partyALanguage,
        partyBLanguage
      );
      const realtimeSecret = await createRealtimeClientSecret(
        activeToken,
        partyALanguage,
        partyBLanguage
      );
      const stream = await mediaDevices.getUserMedia({
        audio: true,
        video: false
      });

      const peer = new RTCPeerConnection();
      const dataChannel = peer.createDataChannel("oai-events") as unknown as DataChannel;

      dataChannel.addEventListener("open", () => {
        setOriginal("Listening...");
        setTranslation(`${languageLabel(partyALanguage)} <-> ${languageLabel(partyBLanguage)} is active.`);
      });
      dataChannel.addEventListener("message", (message) => {
        handleRealtimeEvent(String(message.data));
      });
      dataChannel.addEventListener("error", () => {
        setError("Realtime data channel error");
      });

      stream.getTracks().forEach((track) => {
        peer.addTrack(track, stream);
      });

      const offer = await peer.createOffer({});
      await peer.setLocalDescription(offer);

      const sdpResponse = await fetch(REALTIME_SDP_URL, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${realtimeSecret.value}`,
          "Content-Type": "application/sdp"
        },
        body: offer.sdp
      });

      if (!sdpResponse.ok) {
        throw new Error(await sdpResponse.text());
      }

      const answer = await sdpResponse.text();
      await peer.setRemoteDescription(
        new RTCSessionDescription({
          type: "answer",
          sdp: answer
        })
      );

      peerRef.current = peer;
      mediaStreamRef.current = stream;
      dataChannelRef.current = dataChannel;
      sessionIdRef.current = apiSession.id;
      setSessionId(apiSession.id);
      setEntitlement((current) =>
        current
          ? { ...current, remainingSeconds: apiSession.remainingSeconds }
          : current
      );

      intervalRef.current = setInterval(async () => {
        try {
          const next = await addUsage(activeToken, apiSession.id, USAGE_TICK_SECONDS);
          setEntitlement(next);
          if (next.remainingSeconds <= 0) {
            await stop();
          }
        } catch (err) {
          setError(err instanceof Error ? err.message : "Usage update failed");
          await stop();
        }
      }, USAGE_TICK_SECONDS * 1000);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not start Realtime session");
      await stop();
    } finally {
      setBusy(false);
    }
  }

  function handleRealtimeEvent(raw: string) {
    try {
      const event = JSON.parse(raw) as {
        type?: string;
        delta?: string;
        text?: string;
        transcript?: string;
        error?: { message?: string };
      };

      if (event.type === "error") {
        setError(event.error?.message ?? "OpenAI Realtime error");
        return;
      }

      if (event.type?.includes("input_audio_transcription") && event.transcript) {
        setOriginal(event.transcript);
      }

      if (event.type === "input_audio_buffer.speech_started") {
        setTranslation("Listening...");
      }

      if (event.type === "input_audio_buffer.speech_stopped") {
        setTranslation("Translating...");
      }

      if (event.type === "response.created") {
        setTranslation("Translating...");
      }

      if (event.type === "response.output_audio_transcript.delta" && event.delta) {
        setTranslation((current) =>
          current.startsWith("Speak") ||
          current.startsWith("Realtime") ||
          current === "Listening..." ||
          current === "Translating..."
            ? event.delta ?? ""
            : current + event.delta
        );
      }

      if (event.type === "response.output_audio_transcript.done" && event.transcript) {
        setTranslation(event.transcript);
      }
    } catch {
      // Ignore non-JSON data channel messages.
    }
  }

  async function stop() {
    const activeToken = token;

    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }

    const activeSessionId = sessionIdRef.current;
    sessionIdRef.current = null;
    setSessionId(null);

    dataChannelRef.current?.close();
    dataChannelRef.current = null;

    mediaStreamRef.current?.getTracks().forEach((track) => track.stop());
    mediaStreamRef.current = null;

    peerRef.current?.close();
    peerRef.current = null;

    try {
      if (activeToken && activeSessionId) {
        await endTranslationSession(activeToken, activeSessionId);
      }
      if (activeToken) {
        await refreshEntitlement(activeToken);
      }
      if (activeSessionId) {
        setOriginal("Session stopped.");
        setTranslation("Ready for the next Realtime session.");
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not stop session");
    }
  }

  const remainingMinutes = Math.ceil((entitlement?.remainingSeconds ?? 0) / 60);
  const isActive = Boolean(sessionId);

  return (
    <SafeAreaView style={styles.screen}>
      <ScrollView contentContainerStyle={styles.content}>
        <View style={styles.header}>
          <View>
            <Text style={styles.eyebrow}>Live Translate Platform</Text>
            <Text style={styles.title}>Translate</Text>
          </View>
          <Link href="/dashboard" asChild>
            <Pressable style={styles.iconButton}>
              <Ionicons name="person-circle-outline" size={25} color="#18202f" />
            </Pressable>
          </Link>
        </View>

        <View style={styles.statusRow}>
          <View style={styles.statusPill}>
            <Text style={styles.statusLabel}>Access</Text>
            <Text style={styles.statusValue}>
              {entitlement?.canTranslate ? entitlement.plan : "Blocked"}
            </Text>
          </View>
          <View style={styles.statusPill}>
            <Text style={styles.statusLabel}>Remaining</Text>
            <Text style={styles.statusValue}>{remainingMinutes} min</Text>
          </View>
        </View>

        {error ? <Text style={styles.error}>{error}</Text> : null}

        <View style={styles.panel}>
          <Text style={styles.panelTitle}>Conversation</Text>
          <Text style={styles.controlLabel}>Person A speaks</Text>
          <View style={styles.segmented}>
            {targetLanguages
              .filter((item) => quickLanguages.includes(item.code))
              .map((item) => (
              <Pressable
                disabled={isActive}
                key={item.code}
                onPress={() => setPartyALanguage(item.code)}
                style={[
                  styles.segment,
                  partyALanguage === item.code && styles.segmentActive,
                  isActive && styles.segmentDisabled
                ]}
              >
                <Text
                  style={[
                    styles.segmentText,
                    partyALanguage === item.code && styles.segmentTextActive
                  ]}
                >
                  {item.label}
                </Text>
              </Pressable>
              ))}
          </View>

          <Text style={styles.controlLabel}>Person B speaks</Text>
          <View style={styles.segmented}>
            {targetLanguages
              .filter((item) => quickLanguages.includes(item.code))
              .map((item) => (
                <Pressable
                  disabled={isActive}
                  key={item.code}
                  onPress={() => setPartyBLanguage(item.code)}
                  style={[
                    styles.segment,
                    partyBLanguage === item.code && styles.segmentActive,
                    isActive && styles.segmentDisabled
                  ]}
                >
                  <Text
                    style={[
                      styles.segmentText,
                      partyBLanguage === item.code && styles.segmentTextActive
                    ]}
                  >
                    {item.label}
                  </Text>
                </Pressable>
              ))}
          </View>

          <Option
            label="Interpreting"
            value={`${languageLabel(partyALanguage)} <-> ${languageLabel(partyBLanguage)}`}
          />
          <Option label="Audio source" value="Microphone" />
          <Pressable
            disabled={busy || (!isActive && entitlement?.canTranslate === false)}
            onPress={isActive ? stop : start}
            style={[
              styles.primaryButton,
              isActive && styles.stopButton,
              (busy || (!isActive && entitlement?.canTranslate === false)) &&
                styles.disabledButton
            ]}
          >
            <Ionicons name={isActive ? "stop" : "mic"} size={20} color="#ffffff" />
            <Text style={styles.primaryButtonText}>
              {busy ? "Working..." : isActive ? "Stop" : "Start translating"}
            </Text>
          </Pressable>
        </View>

        <Transcript title="Original" text={original} />
        <Transcript title="Translation" text={translation} />
      </ScrollView>
    </SafeAreaView>
  );
}

function Option({ label, value }: { label: string; value: string }) {
  return (
    <Pressable style={styles.option}>
      <View>
        <Text style={styles.optionLabel}>{label}</Text>
        <Text style={styles.optionValue}>{value}</Text>
      </View>
      <Ionicons name="chevron-forward" size={20} color="#667085" />
    </Pressable>
  );
}

function languageLabel(code: string) {
  return targetLanguages.find((language) => language.code === code)?.label ?? code;
}

function Transcript({ title, text }: { title: string; text: string }) {
  return (
    <View style={styles.transcript}>
      <Text style={styles.transcriptTitle}>{title}</Text>
      <Text style={styles.transcriptText}>{text}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  screen: {
    flex: 1,
    backgroundColor: "#f7f8fb"
  },
  content: {
    padding: 20,
    gap: 14
  },
  header: {
    minHeight: 56,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between"
  },
  eyebrow: {
    color: "#0f766e",
    fontSize: 13,
    fontWeight: "700"
  },
  title: {
    color: "#18202f",
    fontSize: 30,
    fontWeight: "700"
  },
  iconButton: {
    width: 44,
    height: 44,
    alignItems: "center",
    justifyContent: "center",
    borderWidth: 1,
    borderColor: "#d9e0ea",
    borderRadius: 8,
    backgroundColor: "#ffffff"
  },
  statusRow: {
    flexDirection: "row",
    gap: 10
  },
  statusPill: {
    flex: 1,
    borderWidth: 1,
    borderColor: "#d9e0ea",
    borderRadius: 10,
    backgroundColor: "#ffffff",
    padding: 14
  },
  statusLabel: {
    color: "#667085",
    fontSize: 12
  },
  statusValue: {
    marginTop: 4,
    color: "#18202f",
    fontSize: 18,
    fontWeight: "700",
    textTransform: "capitalize"
  },
  panel: {
    borderWidth: 1,
    borderColor: "#d9e0ea",
    borderRadius: 10,
    backgroundColor: "#ffffff",
    padding: 16
  },
  panelTitle: {
    color: "#18202f",
    fontSize: 18,
    fontWeight: "700",
    marginBottom: 8
  },
  option: {
    minHeight: 58,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    borderBottomWidth: 1,
    borderBottomColor: "#eef2f7"
  },
  optionLabel: {
    color: "#667085",
    fontSize: 12
  },
  optionValue: {
    marginTop: 3,
    color: "#18202f",
    fontSize: 16,
    fontWeight: "700"
  },
  controlLabel: {
    marginTop: 8,
    marginBottom: 8,
    color: "#667085",
    fontSize: 12,
    fontWeight: "700"
  },
  segmented: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
    marginBottom: 8
  },
  segment: {
    minWidth: "30%",
    minHeight: 40,
    alignItems: "center",
    justifyContent: "center",
    borderWidth: 1,
    borderColor: "#d9e0ea",
    borderRadius: 8,
    backgroundColor: "#ffffff",
    paddingHorizontal: 8
  },
  segmentActive: {
    borderColor: "#0f766e",
    backgroundColor: "#e7f5f3"
  },
  segmentDisabled: {
    opacity: 0.7
  },
  segmentText: {
    color: "#667085",
    fontSize: 12,
    fontWeight: "700",
    textAlign: "center"
  },
  segmentTextActive: {
    color: "#0f766e"
  },
  primaryButton: {
    minHeight: 52,
    marginTop: 16,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: 8,
    borderRadius: 8,
    backgroundColor: "#0f766e"
  },
  stopButton: {
    backgroundColor: "#b42318"
  },
  disabledButton: {
    opacity: 0.6
  },
  primaryButtonText: {
    color: "#ffffff",
    fontSize: 16,
    fontWeight: "700"
  },
  transcript: {
    minHeight: 150,
    borderWidth: 1,
    borderColor: "#d9e0ea",
    borderRadius: 10,
    backgroundColor: "#ffffff",
    padding: 16
  },
  transcriptTitle: {
    color: "#18202f",
    fontSize: 17,
    fontWeight: "700"
  },
  transcriptText: {
    marginTop: 10,
    color: "#667085",
    fontSize: 16,
    lineHeight: 24
  },
  error: {
    color: "#b42318",
    lineHeight: 20
  }
});
