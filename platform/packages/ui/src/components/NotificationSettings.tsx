import * as React from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";
import { Label } from "./ui/label";
import { Switch } from "./ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./ui/select";
import { Input } from "./ui/input";
import { Bell, Clock, Globe } from "./icons";

/** Digest settings as returned from the API. */
export interface DigestSettings {
  frequency: "daily" | "weekly" | "off";
  quiet_start: string; // HH:MM, e.g. "22:00"
  quiet_end: string; // HH:MM, e.g. "08:00"
  timezone: string; // IANA, e.g. "America/New_York"
}

interface NotificationSettingsProps {
  /** Current digest settings. */
  settings: DigestSettings;
  /** Called when user changes any setting. */
  onChange: (settings: DigestSettings) => void;
  /** Whether a save is in progress. */
  saving?: boolean;
}

/** Common IANA timezones for the dropdown. */
const timezones = [
  "UTC",
  "America/New_York",
  "America/Chicago",
  "America/Denver",
  "America/Los_Angeles",
  "America/Sao_Paulo",
  "Europe/London",
  "Europe/Paris",
  "Europe/Berlin",
  "Europe/Moscow",
  "Asia/Dubai",
  "Asia/Kolkata",
  "Asia/Shanghai",
  "Asia/Tokyo",
  "Asia/Seoul",
  "Australia/Sydney",
  "Pacific/Auckland",
];

/**
 * Notification preferences panel — digest frequency, quiet hours, and timezone.
 *
 * This is a controlled component: it calls `onChange` with the full updated
 * settings object whenever the user modifies a value. The parent is responsible
 * for persisting via the API.
 */
export function NotificationSettings({ settings, onChange, saving }: NotificationSettingsProps) {
  const quietEnabled = settings.quiet_start !== "" && settings.quiet_end !== "";

  function update(patch: Partial<DigestSettings>) {
    onChange({ ...settings, ...patch });
  }

  function toggleQuietHours(enabled: boolean) {
    if (enabled) {
      update({ quiet_start: "22:00", quiet_end: "08:00" });
    } else {
      update({ quiet_start: "", quiet_end: "" });
    }
  }

  return (
    <div className="flex flex-col gap-4">
      {/* ── Digest frequency ──────────────────────────── */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Bell className="w-4 h-4" />
            Email digest
          </CardTitle>
          <CardDescription>
            Receive a summary of notifications by email.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-4">
            <Label htmlFor="digest-frequency" className="min-w-24">
              Frequency
            </Label>
            <Select
              value={settings.frequency}
              onValueChange={(v) => update({ frequency: v as DigestSettings["frequency"] })}
            >
              <SelectTrigger id="digest-frequency" className="w-40">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="daily">Daily</SelectItem>
                <SelectItem value="weekly">Weekly</SelectItem>
                <SelectItem value="off">Off</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      {/* ── Quiet hours ───────────────────────────────── */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Clock className="w-4 h-4" />
            Quiet hours
          </CardTitle>
          <CardDescription>
            Suppress non-urgent notifications during these hours. High-priority alerts always deliver immediately.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex items-center gap-4">
            <Label htmlFor="quiet-toggle" className="min-w-24">
              Enabled
            </Label>
            <Switch
              id="quiet-toggle"
              checked={quietEnabled}
              onCheckedChange={toggleQuietHours}
            />
          </div>

          {quietEnabled && (
            <>
              <div className="flex items-center gap-4">
                <Label htmlFor="quiet-start" className="min-w-24">
                  From
                </Label>
                <Input
                  id="quiet-start"
                  type="time"
                  value={settings.quiet_start}
                  onChange={(e) => update({ quiet_start: e.target.value })}
                  className="w-32"
                />
              </div>
              <div className="flex items-center gap-4">
                <Label htmlFor="quiet-end" className="min-w-24">
                  Until
                </Label>
                <Input
                  id="quiet-end"
                  type="time"
                  value={settings.quiet_end}
                  onChange={(e) => update({ quiet_end: e.target.value })}
                  className="w-32"
                />
              </div>
            </>
          )}
        </CardContent>
      </Card>

      {/* ── Timezone ──────────────────────────────────── */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Globe className="w-4 h-4" />
            Timezone
          </CardTitle>
          <CardDescription>
            Used for quiet hours and digest scheduling.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-4">
            <Label htmlFor="timezone" className="min-w-24">
              Timezone
            </Label>
            <Select
              value={settings.timezone}
              onValueChange={(v) => update({ timezone: v })}
            >
              <SelectTrigger id="timezone" className="w-56">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {timezones.map((tz) => (
                  <SelectItem key={tz} value={tz}>
                    {tz.replace(/_/g, " ")}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      {saving && (
        <p className="text-xs text-muted-foreground">Saving...</p>
      )}
    </div>
  );
}
