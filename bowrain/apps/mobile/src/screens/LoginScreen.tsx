import React, { useState } from "react";
import {
  View,
  Text,
  TextInput,
  TouchableOpacity,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { useAuth } from "../auth/AuthContext";
import type { KeycloakConfig } from "../auth/keycloak";

export function LoginScreen() {
  const { login } = useAuth();
  const [serverUrl, setServerUrl] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleLogin = async () => {
    if (!serverUrl.trim()) {
      setError("Enter your Bowrain server URL");
      return;
    }

    setLoading(true);
    setError(null);

    try {
      // Fetch server config to get Keycloak details.
      const resp = await fetch(`${serverUrl.trim()}/api/v1/config`);
      if (!resp.ok) throw new Error("Could not connect to server");
      const config = await resp.json();

      const keycloakConfig: KeycloakConfig = {
        serverUrl: serverUrl.trim(),
        keycloakUrl: config.oidc_issuer_url?.replace(/\/realms\/.*$/, "") ?? "",
        realm: config.oidc_realm ?? "bowrain",
        clientId: config.oidc_client_id ?? "bowrain-mobile",
      };

      const success = await login(serverUrl.trim(), keycloakConfig);
      if (!success) {
        setError("Login cancelled or failed");
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Connection failed");
    } finally {
      setLoading(false);
    }
  };

  return (
    <View style={styles.container}>
      <View style={styles.logoArea}>
        <Text style={styles.logoText}>Bowrain</Text>
        <Text style={styles.tagline}>Localization Review</Text>
      </View>

      <View style={styles.form}>
        <Text style={styles.label}>Server URL</Text>
        <TextInput
          style={styles.input}
          placeholder="https://bowrain.example.com"
          placeholderTextColor="#484f58"
          value={serverUrl}
          onChangeText={setServerUrl}
          autoCapitalize="none"
          autoCorrect={false}
          keyboardType="url"
        />

        {error && <Text style={styles.error}>{error}</Text>}

        <TouchableOpacity
          style={[styles.button, loading && styles.buttonDisabled]}
          onPress={handleLogin}
          disabled={loading}
        >
          {loading ? (
            <ActivityIndicator color="#fff" />
          ) : (
            <Text style={styles.buttonText}>Connect & Sign In</Text>
          )}
        </TouchableOpacity>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#0d1117",
    justifyContent: "center",
    padding: 24,
  },
  logoArea: { alignItems: "center", marginBottom: 48 },
  logoText: {
    fontSize: 36,
    fontWeight: "800",
    color: "#c27930",
    letterSpacing: 1,
  },
  tagline: { fontSize: 14, color: "#8b949e", marginTop: 4 },
  form: {},
  label: {
    fontSize: 13,
    fontWeight: "600",
    color: "#8b949e",
    marginBottom: 8,
    textTransform: "uppercase",
    letterSpacing: 0.5,
  },
  input: {
    backgroundColor: "#161b22",
    borderWidth: 1,
    borderColor: "#30363d",
    borderRadius: 10,
    padding: 14,
    fontSize: 16,
    color: "#e6edf3",
    marginBottom: 16,
  },
  error: {
    color: "#f85149",
    fontSize: 13,
    marginBottom: 12,
  },
  button: {
    backgroundColor: "#c27930",
    borderRadius: 10,
    padding: 16,
    alignItems: "center",
  },
  buttonDisabled: { opacity: 0.6 },
  buttonText: {
    color: "#fff",
    fontSize: 16,
    fontWeight: "700",
  },
});
