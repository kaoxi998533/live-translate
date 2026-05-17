import { Link, Redirect } from "expo-router";
import { useEffect, useState } from "react";
import { Pressable, ScrollView, StyleSheet, Text, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { getEntitlement, type Entitlement } from "../lib/api";
import { useAuth } from "../lib/auth";

export default function DashboardScreen() {
  const { token, user, signOut } = useAuth();
  const [entitlement, setEntitlement] = useState<Entitlement | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!token) {
      return;
    }
    getEntitlement(token)
      .then(setEntitlement)
      .catch((err) => setError(err instanceof Error ? err.message : "Could not load usage"));
  }, [token]);

  if (!token) {
    return <Redirect href="/login" />;
  }

  const usedMinutes = Math.ceil((entitlement?.usedSeconds ?? 0) / 60);
  const limitMinutes = Math.ceil((entitlement?.weeklyLimitSeconds ?? 0) / 60);
  const remainingMinutes = Math.ceil((entitlement?.remainingSeconds ?? 0) / 60);

  return (
    <SafeAreaView style={styles.screen}>
      <ScrollView contentContainerStyle={styles.content}>
        <View style={styles.header}>
          <View>
            <Text style={styles.title}>Dashboard</Text>
            <Text style={styles.subtitle}>{user?.email}</Text>
          </View>
          <Link href="/translate" asChild>
            <Pressable style={styles.secondaryButton}>
              <Text style={styles.secondaryButtonText}>Translate</Text>
            </Pressable>
          </Link>
        </View>

        <View style={styles.metrics}>
          <Metric label="Plan" value={entitlement?.plan ?? "Loading"} />
          <Metric label="Weekly usage" value={`${usedMinutes} / ${limitMinutes} min`} />
          <Metric label="Remaining" value={`${remainingMinutes} min`} />
        </View>

        {error ? <Text style={styles.error}>{error}</Text> : null}

        <View style={styles.panel}>
          <Text style={styles.panelTitle}>Billing</Text>
          <Text style={styles.bodyText}>
            Trial and weekly quota enforcement are active in the local Go API.
            Stripe is scaffolded and ready for real product credentials.
          </Text>
        </View>

        <Pressable onPress={signOut} style={styles.signOutButton}>
          <Text style={styles.signOutText}>Sign out</Text>
        </Pressable>
      </ScrollView>
    </SafeAreaView>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.metric}>
      <Text style={styles.metricLabel}>{label}</Text>
      <Text style={styles.metricValue}>{value}</Text>
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
    minHeight: 48,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between"
  },
  title: {
    color: "#18202f",
    fontSize: 28,
    fontWeight: "700"
  },
  subtitle: {
    marginTop: 2,
    color: "#667085",
    fontSize: 13
  },
  metrics: {
    gap: 10
  },
  metric: {
    borderWidth: 1,
    borderColor: "#d9e0ea",
    borderRadius: 10,
    backgroundColor: "#ffffff",
    padding: 16
  },
  metricLabel: {
    color: "#667085",
    fontSize: 13
  },
  metricValue: {
    marginTop: 8,
    color: "#18202f",
    fontSize: 24,
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
    fontWeight: "700"
  },
  bodyText: {
    marginTop: 8,
    color: "#667085",
    fontSize: 15,
    lineHeight: 22
  },
  secondaryButton: {
    minHeight: 40,
    justifyContent: "center",
    borderWidth: 1,
    borderColor: "#d9e0ea",
    borderRadius: 8,
    paddingHorizontal: 14,
    backgroundColor: "#ffffff"
  },
  secondaryButtonText: {
    color: "#18202f",
    fontWeight: "700"
  },
  signOutButton: {
    minHeight: 46,
    alignItems: "center",
    justifyContent: "center",
    borderWidth: 1,
    borderColor: "#d9e0ea",
    borderRadius: 8,
    backgroundColor: "#ffffff"
  },
  signOutText: {
    color: "#b42318",
    fontWeight: "700"
  },
  error: {
    color: "#b42318"
  }
});
