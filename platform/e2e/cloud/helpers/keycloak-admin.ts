/**
 * Keycloak admin API client for e2e test support.
 * Used to verify email addresses and manage test users.
 */

const REALM = "bowrain";

export interface KeycloakAdminConfig {
  /** Keycloak base URL, e.g. https://auth.dev.bowrain.cloud */
  baseUrl: string;
  /** Admin password (username is always "admin") */
  adminPassword: string;
}

export class KeycloakAdmin {
  private config: KeycloakAdminConfig;
  private token: string | null = null;
  private tokenExpiry = 0;

  constructor(config: KeycloakAdminConfig) {
    this.config = config;
  }

  /** Obtain an admin access token (cached until near expiry). */
  private async getToken(): Promise<string> {
    if (this.token && Date.now() < this.tokenExpiry - 10_000) {
      return this.token;
    }

    const resp = await fetch(
      `${this.config.baseUrl}/realms/master/protocol/openid-connect/token`,
      {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({
          client_id: "admin-cli",
          username: "admin",
          password: this.config.adminPassword,
          grant_type: "password",
        }),
      },
    );

    if (!resp.ok) {
      throw new Error(`Keycloak admin auth failed: ${resp.status} ${await resp.text()}`);
    }

    const data = await resp.json();
    this.token = data.access_token;
    this.tokenExpiry = Date.now() + data.expires_in * 1000;
    return this.token!;
  }

  private async adminGet(path: string) {
    const token = await this.getToken();
    const resp = await fetch(`${this.config.baseUrl}/admin/realms/${REALM}${path}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!resp.ok) throw new Error(`Admin GET ${path}: ${resp.status}`);
    return resp.json();
  }

  private async adminPut(path: string, body: unknown) {
    const token = await this.getToken();
    const resp = await fetch(`${this.config.baseUrl}/admin/realms/${REALM}${path}`, {
      method: "PUT",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify(body),
    });
    if (!resp.ok && resp.status !== 204) {
      throw new Error(`Admin PUT ${path}: ${resp.status} ${await resp.text()}`);
    }
  }

  private async adminDelete(path: string) {
    const token = await this.getToken();
    const resp = await fetch(`${this.config.baseUrl}/admin/realms/${REALM}${path}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!resp.ok && resp.status !== 204) {
      throw new Error(`Admin DELETE ${path}: ${resp.status}`);
    }
  }

  /** Find a user by email. Returns null if not found. */
  async findUser(email: string): Promise<KeycloakUser | null> {
    const users: KeycloakUser[] = await this.adminGet(
      `/users?email=${encodeURIComponent(email)}&exact=true`,
    );
    return users.length > 0 ? users[0] : null;
  }

  /** Mark a user's email as verified (keeps other required actions like passkey enrollment). */
  async verifyEmail(userId: string): Promise<void> {
    const user: KeycloakUser = await this.adminGet(`/users/${userId}`);
    // Remove only VERIFY_EMAIL, preserve webauthn-register-passwordless and others.
    const remainingActions = user.requiredActions.filter((a) => a !== "VERIFY_EMAIL");
    await this.adminPut(`/users/${userId}`, {
      emailVerified: true,
      requiredActions: remainingActions,
    });
  }

  /** Verify email and clear ALL required actions (skip passkey enrollment). */
  async activateUser(email: string): Promise<void> {
    const user = await this.findUser(email);
    if (!user) throw new Error(`User not found: ${email}`);
    await this.adminPut(`/users/${user.id}`, {
      emailVerified: true,
      requiredActions: [],
    });
  }

  /** Delete a user by ID. */
  async deleteUser(userId: string): Promise<void> {
    await this.adminDelete(`/users/${userId}`);
  }

  /** Delete a user by email. No-op if user doesn't exist. */
  async deleteUserByEmail(email: string): Promise<void> {
    const user = await this.findUser(email);
    if (user) await this.deleteUser(user.id);
  }

  /** Check if Keycloak is reachable. */
  async isReady(): Promise<boolean> {
    try {
      const resp = await fetch(`${this.config.baseUrl}/realms/${REALM}`);
      return resp.ok;
    } catch {
      return false;
    }
  }
}

interface KeycloakUser {
  id: string;
  username: string;
  email: string;
  emailVerified: boolean;
  firstName: string;
  lastName: string;
  requiredActions: string[];
}
