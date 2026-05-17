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
  send: (data: string) => void;
  addEventListener: (type: string, listener: (event: { data?: unknown }) => void) => void;
};

type AudioTrack = {
  enabled: boolean;
  stop: () => void;
};

export default function TranslateScreen() {
  const { token } = useAuth();
  const [entitlement, setEntitlement] = useState<Entitlement | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [original, setOriginal] = useState("开始会话后，把手机放在两位说话者中间。");
  const [translation, setTranslation] = useState("应用会把每个人的话翻译成对方的语言。");
  const [partyALanguage, setPartyALanguage] = useState("zh");
  const [partyBLanguage, setPartyBLanguage] = useState("en");
  const [listenMode, setListenMode] = useState<"auto" | "hold">("auto");
  const [holding, setHolding] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const sessionIdRef = useRef<string | null>(null);
  const peerRef = useRef<RTCPeerConnection | null>(null);
  const mediaStreamRef = useRef<MediaStream | null>(null);
  const dataChannelRef = useRef<DataChannel | null>(null);
  const micTrackRef = useRef<AudioTrack | null>(null);

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
      setError(err instanceof Error ? err.message : "无法读取使用权限");
    }
  }

  async function start() {
    if (!token) {
      return;
    }
    const activeToken = token;

    setBusy(true);
    setError(null);
    setOriginal("正在连接麦克风...");
    setTranslation("正在连接实时翻译...");

    try {
      const apiSession = await createTranslationSession(
        activeToken,
        partyALanguage,
        partyBLanguage
      );
      const realtimeSecret = await createRealtimeClientSecret(
        activeToken,
        partyALanguage,
        partyBLanguage,
        listenMode
      );
      const stream = await mediaDevices.getUserMedia({
        audio: true,
        video: false
      });

      const peer = new RTCPeerConnection();
      const dataChannel = peer.createDataChannel("oai-events") as unknown as DataChannel;

      dataChannel.addEventListener("open", () => {
        setOriginal("正在监听...");
        setTranslation(`${languageLabel(partyALanguage)} ↔ ${languageLabel(partyBLanguage)} 已连接。`);
      });
      dataChannel.addEventListener("message", (message) => {
        handleRealtimeEvent(String(message.data));
      });
      dataChannel.addEventListener("error", () => {
        setError("实时通道错误");
      });

      stream.getTracks().forEach((track) => {
        if (listenMode === "hold") {
          track.enabled = false;
        }
        micTrackRef.current = track as unknown as AudioTrack;
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
          setError(err instanceof Error ? err.message : "用量更新失败");
          await stop();
        }
      }, USAGE_TICK_SECONDS * 1000);
    } catch (err) {
      setError(err instanceof Error ? err.message : "无法启动实时翻译");
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
        setError(event.error?.message ?? "实时翻译错误");
        return;
      }

      if (event.type?.includes("input_audio_transcription") && event.transcript) {
        setOriginal(event.transcript);
      }

      if (event.type === "input_audio_buffer.speech_started") {
        setTranslation("正在听...");
      }

      if (event.type === "input_audio_buffer.speech_stopped") {
        setTranslation("正在翻译...");
      }

      if (event.type === "response.created") {
        setTranslation("正在翻译...");
      }

      if (event.type === "response.output_audio_transcript.delta" && event.delta) {
        setTranslation((current) =>
          current.startsWith("说话") ||
          current.startsWith("实时") ||
          current === "正在听..." ||
          current === "正在翻译..."
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
    micTrackRef.current = null;
    setHolding(false);

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
        setOriginal("会话已结束。");
        setTranslation("可以开始下一次同声翻译。");
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "无法结束会话");
    }
  }

  function beginHold() {
    if (listenMode !== "hold" || !sessionId || busy) {
      return;
    }
    micTrackRef.current && (micTrackRef.current.enabled = true);
    setHolding(true);
    setOriginal("按住时正在收音...");
    setTranslation("松开后翻译。");
  }

  function endHold() {
    if (listenMode !== "hold" || !sessionId) {
      return;
    }
    micTrackRef.current && (micTrackRef.current.enabled = false);
    setHolding(false);
    setTranslation("正在翻译...");
    sendRealtimeEvent({ type: "input_audio_buffer.commit" });
    sendRealtimeEvent({ type: "response.create" });
  }

  function sendRealtimeEvent(event: Record<string, unknown>) {
    dataChannelRef.current?.send(JSON.stringify(event));
  }

  const remainingMinutes = Math.ceil((entitlement?.remainingSeconds ?? 0) / 60);
  const isActive = Boolean(sessionId);

  return (
    <SafeAreaView style={styles.screen}>
      <ScrollView contentContainerStyle={styles.content}>
        <View style={styles.header}>
          <View>
            <Text style={styles.eyebrow}>Live Translate Platform</Text>
            <Text style={styles.title}>同声翻译</Text>
          </View>
          <Link href="/dashboard" asChild>
            <Pressable style={styles.iconButton}>
              <Ionicons name="person-circle-outline" size={25} color="#18202f" />
            </Pressable>
          </Link>
        </View>

        <View style={styles.statusRow}>
          <View style={styles.statusPill}>
            <Text style={styles.statusLabel}>权限</Text>
            <Text style={styles.statusValue}>
              {entitlement?.canTranslate ? planLabel(entitlement.plan) : "不可用"}
            </Text>
          </View>
          <View style={styles.statusPill}>
            <Text style={styles.statusLabel}>剩余</Text>
            <Text style={styles.statusValue}>{remainingMinutes} 分钟</Text>
          </View>
        </View>

        {error ? <Text style={styles.error}>{error}</Text> : null}

        <View style={styles.panel}>
          <Text style={styles.panelTitle}>对话设置</Text>
          <Text style={styles.controlLabel}>收音方式</Text>
          <View style={styles.modeSwitch}>
            <Pressable
              disabled={isActive}
              onPress={() => setListenMode("auto")}
              style={[
                styles.modeOption,
                listenMode === "auto" && styles.modeOptionActive,
                isActive && styles.segmentDisabled
              ]}
            >
              <Text
                style={[
                  styles.modeOptionText,
                  listenMode === "auto" && styles.modeOptionTextActive
                ]}
              >
                自动监听
              </Text>
            </Pressable>
            <Pressable
              disabled={isActive}
              onPress={() => setListenMode("hold")}
              style={[
                styles.modeOption,
                listenMode === "hold" && styles.modeOptionActive,
                isActive && styles.segmentDisabled
              ]}
            >
              <Text
                style={[
                  styles.modeOptionText,
                  listenMode === "hold" && styles.modeOptionTextActive
                ]}
              >
                按住说话
              </Text>
            </Pressable>
          </View>
          <Text style={styles.controlLabel}>A 说的语言</Text>
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

          <Text style={styles.controlLabel}>B 说的语言</Text>
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
            label="翻译方向"
            value={`${languageLabel(partyALanguage)} ↔ ${languageLabel(partyBLanguage)}`}
          />
          <Option label="音频来源" value="手机麦克风" />
          {listenMode === "hold" && isActive ? (
            <View style={styles.holdControls}>
              <Pressable
                onPressIn={beginHold}
                onPressOut={endHold}
                style={[styles.primaryButton, holding && styles.recordingButton]}
              >
                <Ionicons name="mic" size={20} color="#ffffff" />
                <Text style={styles.primaryButtonText}>
                  {holding ? "松开翻译" : "按住说话"}
                </Text>
              </Pressable>
              <Pressable onPress={stop} style={[styles.primaryButton, styles.stopButton]}>
                <Ionicons name="stop" size={20} color="#ffffff" />
                <Text style={styles.primaryButtonText}>结束会话</Text>
              </Pressable>
            </View>
          ) : (
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
                {busy
                  ? "处理中..."
                  : isActive
                    ? "停止自动监听"
                    : listenMode === "hold"
                      ? "开始按住说话"
                      : "开始自动监听"}
              </Text>
            </Pressable>
          )}
        </View>

        <Transcript title="原文" text={original} />
        <Transcript title="译文" text={translation} />
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

function planLabel(plan?: string) {
  if (plan === "premium") {
    return "高级会员";
  }
  if (plan === "trial") {
    return "试用中";
  }
  return "读取中";
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
  modeSwitch: {
    flexDirection: "row",
    gap: 8,
    marginBottom: 8
  },
  modeOption: {
    flex: 1,
    minHeight: 44,
    alignItems: "center",
    justifyContent: "center",
    borderWidth: 1,
    borderColor: "#d9e0ea",
    borderRadius: 8,
    backgroundColor: "#ffffff",
    paddingHorizontal: 10
  },
  modeOptionActive: {
    borderColor: "#0f766e",
    backgroundColor: "#e7f5f3"
  },
  modeOptionText: {
    color: "#667085",
    fontSize: 14,
    fontWeight: "700"
  },
  modeOptionTextActive: {
    color: "#0f766e"
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
  recordingButton: {
    backgroundColor: "#b45309"
  },
  holdControls: {
    gap: 10
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
