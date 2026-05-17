import { Link, Redirect } from "expo-router";
import { useEffect, useState } from "react";
import { Pressable, ScrollView, StyleSheet, Text, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import {
  createPaymentOrder,
  getEntitlement,
  markPaymentOrderPaidForDev,
  type Entitlement,
  type PaymentOrder
} from "../lib/api";
import { useAuth } from "../lib/auth";

export default function DashboardScreen() {
  const { token, user, signOut } = useAuth();
  const [entitlement, setEntitlement] = useState<Entitlement | null>(null);
  const [order, setOrder] = useState<PaymentOrder | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [billingBusy, setBillingBusy] = useState(false);

  useEffect(() => {
    if (!token) {
      return;
    }
    getEntitlement(token)
      .then(setEntitlement)
      .catch((err) => setError(err instanceof Error ? err.message : "无法读取用量"));
  }, [token]);

  if (!token) {
    return <Redirect href="/login" />;
  }

  const usedMinutes = Math.ceil((entitlement?.usedSeconds ?? 0) / 60);
  const limitMinutes = Math.ceil((entitlement?.weeklyLimitSeconds ?? 0) / 60);
  const remainingMinutes = Math.ceil((entitlement?.remainingSeconds ?? 0) / 60);

  async function startWechatPay() {
    if (!token) {
      return;
    }
    setBillingBusy(true);
    setError(null);
    try {
      setOrder(await createPaymentOrder(token, "wechat_pay"));
    } catch (err) {
      setError(err instanceof Error ? err.message : "无法创建支付订单");
    } finally {
      setBillingBusy(false);
    }
  }

  async function markPaidForLocalTest() {
    if (!token || !order) {
      return;
    }
    setBillingBusy(true);
    setError(null);
    try {
      await markPaymentOrderPaidForDev(token, order.id);
      setOrder({ ...order, status: "paid" });
      setEntitlement(await getEntitlement(token));
    } catch (err) {
      setError(err instanceof Error ? err.message : "无法更新订单状态");
    } finally {
      setBillingBusy(false);
    }
  }

  return (
    <SafeAreaView style={styles.screen}>
      <ScrollView contentContainerStyle={styles.content}>
        <View style={styles.header}>
          <View>
            <Text style={styles.title}>我的账号</Text>
            <Text style={styles.subtitle}>{user?.email}</Text>
          </View>
          <Link href="/translate" asChild>
            <Pressable style={styles.secondaryButton}>
              <Text style={styles.secondaryButtonText}>去翻译</Text>
            </Pressable>
          </Link>
        </View>

        <View style={styles.metrics}>
          <Metric label="会员状态" value={planLabel(entitlement?.plan)} />
          <Metric label="本周已用" value={`${usedMinutes} / ${limitMinutes} 分钟`} />
          <Metric label="剩余额度" value={`${remainingMinutes} 分钟`} />
        </View>

        {error ? <Text style={styles.error}>{error}</Text> : null}

        <View style={styles.panel}>
          <Text style={styles.panelTitle}>支付与订阅</Text>
          <Text style={styles.bodyText}>
            当前支持试用额度和每周用量限制。商业版本将接入微信支付、App Store/Google Play
            内购，以及海外 Stripe 支付。
          </Text>
          <Pressable
            disabled={billingBusy}
            onPress={startWechatPay}
            style={[styles.primaryButton, billingBusy && styles.disabledButton]}
          >
            <Text style={styles.primaryButtonText}>
              {billingBusy ? "处理中..." : "微信支付开通高级会员"}
            </Text>
          </Pressable>
          {order ? (
            <View style={styles.orderBox}>
              <Text style={styles.orderText}>订单：{order.providerOrderId}</Text>
              <Text style={styles.orderText}>
                金额：{(order.amountMinor / 100).toFixed(2)} {order.currency}
              </Text>
              <Text style={styles.orderText}>状态：{orderStatusLabel(order.status)}</Text>
              {order.status === "pending" ? (
                <Pressable
                  disabled={billingBusy}
                  onPress={markPaidForLocalTest}
                  style={styles.devButton}
                >
                  <Text style={styles.devButtonText}>本地测试：标记为已支付</Text>
                </Pressable>
              ) : null}
            </View>
          ) : null}
        </View>

        <Pressable onPress={signOut} style={styles.signOutButton}>
          <Text style={styles.signOutText}>退出登录</Text>
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

function planLabel(plan?: string) {
  if (plan === "premium") {
    return "高级会员";
  }
  if (plan === "trial") {
    return "试用中";
  }
  return "读取中";
}

function orderStatusLabel(status: string) {
  if (status === "paid") {
    return "已支付";
  }
  if (status === "pending") {
    return "待支付";
  }
  if (status === "failed") {
    return "支付失败";
  }
  if (status === "canceled") {
    return "已取消";
  }
  return "未知";
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
  primaryButton: {
    minHeight: 46,
    marginTop: 14,
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
    fontWeight: "700"
  },
  orderBox: {
    marginTop: 12,
    borderWidth: 1,
    borderColor: "#d9e0ea",
    borderRadius: 8,
    padding: 12,
    backgroundColor: "#f7f8fb"
  },
  orderText: {
    color: "#18202f",
    lineHeight: 22
  },
  devButton: {
    minHeight: 40,
    marginTop: 10,
    alignItems: "center",
    justifyContent: "center",
    borderRadius: 8,
    backgroundColor: "#18202f"
  },
  devButtonText: {
    color: "#ffffff",
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
