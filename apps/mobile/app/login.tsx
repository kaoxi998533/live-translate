import { useRouter } from "expo-router";
import { useState } from "react";
import { Pressable, StyleSheet, Text, TextInput, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { login, register } from "../lib/api";
import { useAuth } from "../lib/auth";

export default function LoginScreen() {
  const router = useRouter();
  const auth = useAuth();
  const [mode, setMode] = useState<"login" | "register">("register");
  const [email, setEmail] = useState("demo@example.com");
  const [password, setPassword] = useState("password123");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function submit() {
    setError(null);
    setLoading(true);
    try {
      const session =
        mode === "register"
          ? await register(email.trim(), password)
          : await login(email.trim(), password);
      auth.setSession(session);
      router.replace("/translate");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not continue");
    } finally {
      setLoading(false);
    }
  }

  return (
    <SafeAreaView style={styles.screen}>
      <View style={styles.card}>
        <Text style={styles.title}>
          {mode === "register" ? "Create account" : "Sign in"}
        </Text>
        <Text style={styles.subtitle}>Android MVP uses the local Go API.</Text>

        <Text style={styles.label}>Email</Text>
        <TextInput
          autoCapitalize="none"
          keyboardType="email-address"
          onChangeText={setEmail}
          placeholder="you@example.com"
          style={styles.input}
          value={email}
        />

        <Text style={styles.label}>Password</Text>
        <TextInput
          onChangeText={setPassword}
          placeholder="Password"
          secureTextEntry
          style={styles.input}
          value={password}
        />

        {error ? <Text style={styles.error}>{error}</Text> : null}

        <Pressable
          disabled={loading}
          onPress={submit}
          style={[styles.primaryButton, loading && styles.disabledButton]}
        >
          <Text style={styles.primaryButtonText}>
            {loading ? "Working..." : "Continue"}
          </Text>
        </Pressable>

        <Pressable
          onPress={() => setMode(mode === "register" ? "login" : "register")}
          style={styles.textButton}
        >
          <Text style={styles.textButtonLabel}>
            {mode === "register"
              ? "Already have an account? Sign in"
              : "Need an account? Create one"}
          </Text>
        </Pressable>
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: {
    flex: 1,
    justifyContent: "center",
    padding: 20,
    backgroundColor: "#f7f8fb"
  },
  card: {
    borderWidth: 1,
    borderColor: "#d9e0ea",
    borderRadius: 12,
    backgroundColor: "#ffffff",
    padding: 20
  },
  title: {
    color: "#18202f",
    fontSize: 28,
    fontWeight: "700"
  },
  subtitle: {
    marginTop: 6,
    marginBottom: 22,
    color: "#667085",
    fontSize: 15
  },
  label: {
    marginBottom: 7,
    color: "#667085",
    fontSize: 13
  },
  input: {
    minHeight: 48,
    marginBottom: 14,
    borderWidth: 1,
    borderColor: "#d9e0ea",
    borderRadius: 8,
    paddingHorizontal: 12,
    color: "#18202f"
  },
  error: {
    marginBottom: 12,
    color: "#b42318",
    fontSize: 14
  },
  primaryButton: {
    minHeight: 48,
    alignItems: "center",
    justifyContent: "center",
    borderRadius: 8,
    backgroundColor: "#0f766e"
  },
  disabledButton: {
    opacity: 0.65
  },
  primaryButtonText: {
    color: "#ffffff",
    fontSize: 16,
    fontWeight: "700"
  },
  textButton: {
    alignItems: "center",
    paddingTop: 16
  },
  textButtonLabel: {
    color: "#0f766e",
    fontWeight: "700"
  }
});
