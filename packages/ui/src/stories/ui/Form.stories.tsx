import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  FormItem,
  FormLabel,
  FormDescription,
  FormMessage,
  FormControl,
  FormToggle,
  FormInputAction,
  FormFieldGroup,
  FormHelpText,
} from "../../components/ui/form";
import { Input } from "../../components/ui/input";
import { Button } from "../../components/ui/button";
import { Switch } from "../../components/ui/switch";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "../../components/ui/select";

const meta: Meta = {
  title: "Foundations/Form Primitives",
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Composable form layout primitives. Use these to build any form UI — from simple config editors to complex schema-driven forms. Follows the shadcn naming convention (FormItem, FormLabel, etc.) without requiring react-hook-form.",
      },
    },
  },
};

export default meta;

// ── FormItem + FormLabel + FormDescription + FormMessage ─────────────

export const BasicField: StoryObj = {
  name: "FormItem — Basic Field",
  render: () => {
    const [value, setValue] = useState("");
    return (
      <div className="max-w-xs">
        <FormItem>
          <FormLabel>Project Name</FormLabel>
          <FormDescription>A short identifier for your project.</FormDescription>
          <FormControl>
            <Input
              value={value}
              placeholder="my-project"
              onChange={(e) => setValue(e.target.value)}
            />
          </FormControl>
        </FormItem>
      </div>
    );
  },
};

export const FieldWithError: StoryObj = {
  name: "FormItem — With Validation Error",
  render: () => (
    <div className="max-w-xs">
      <FormItem>
        <FormLabel>Email</FormLabel>
        <FormControl>
          <Input defaultValue="not-an-email" className="border-destructive" />
        </FormControl>
        <FormMessage>Please enter a valid email address.</FormMessage>
      </FormItem>
    </div>
  ),
};

export const FieldWithModifiedIndicator: StoryObj = {
  name: "FormItem — Modified from Preset",
  render: () => (
    <div className="max-w-xs space-y-2">
      <FormItem modified>
        <FormLabel>Threshold</FormLabel>
        <FormDescription>Minimum match score (0–100). Value differs from preset.</FormDescription>
        <FormControl>
          <Input type="number" defaultValue="85" />
        </FormControl>
      </FormItem>
      <FormItem>
        <FormLabel>Max Results</FormLabel>
        <FormDescription>Unmodified field for comparison.</FormDescription>
        <FormControl>
          <Input type="number" defaultValue="100" />
        </FormControl>
      </FormItem>
    </div>
  ),
};

export const DisabledField: StoryObj = {
  name: "FormItem — Disabled",
  render: () => (
    <div className="max-w-xs">
      <FormItem disabled>
        <FormLabel disabled>Output Path</FormLabel>
        <FormDescription>Disabled when auto-detect is on.</FormDescription>
        <FormControl>
          <Input defaultValue="/output/report.html" disabled />
        </FormControl>
      </FormItem>
    </div>
  ),
};

export const FieldWithHelp: StoryObj = {
  name: "FormItem — With Expandable Help",
  render: () => (
    <div className="max-w-xs">
      <FormItem>
        <FormLabel>Extraction Rules</FormLabel>
        <FormControl>
          <Input defaultValue=".*" className="font-mono" />
        </FormControl>
        <FormHelpText
          description="Regex patterns that determine which JSON paths are extracted for translation."
          notes={[
            "Patterns are matched against the full JSON path (e.g., $.messages[*].text).",
            "Use .* to extract all string values.",
          ]}
          dependencies={[
            { property: "extractAll", condition: "must be false" },
          ]}
        />
      </FormItem>
    </div>
  ),
};

// ── FormToggle ───────────────────────────────────────────────────────

export const Toggle: StoryObj = {
  name: "FormToggle — Boolean Field",
  render: () => {
    const [checked, setChecked] = useState(true);
    return (
      <div className="max-w-xs space-y-2">
        <FormToggle
          checked={checked}
          onCheckedChange={setChecked}
          label="Check Leading Whitespace"
          description="Flag text units where leading whitespace differs between source and target."
        />
        <FormToggle
          checked={false}
          onCheckedChange={() => {}}
          label="Disabled Toggle"
          description="This toggle is disabled."
          disabled
        />
        <FormToggle
          checked={true}
          onCheckedChange={() => {}}
          label="Modified Toggle"
          description="Value differs from active preset."
          modified
        />
      </div>
    );
  },
};

export const ToggleCompact: StoryObj = {
  name: "FormToggle — Compact Mode",
  render: () => {
    const [a, setA] = useState(true);
    const [b, setB] = useState(false);
    const [c, setC] = useState(true);
    return (
      <div className="max-w-xs space-y-0.5">
        <FormToggle checked={a} onCheckedChange={setA} label="Leading whitespace" compact />
        <FormToggle checked={b} onCheckedChange={setB} label="Trailing whitespace" compact />
        <FormToggle checked={c} onCheckedChange={setC} label="Empty target" compact />
      </div>
    );
  },
};

// ── FormInputAction ──────────────────────────────────────────────────

export const InputWithAction: StoryObj = {
  name: "FormInputAction — Path Picker",
  render: () => (
    <div className="max-w-sm">
      <FormItem>
        <FormLabel>Report File Path</FormLabel>
        <FormDescription>Path of the report file to generate.</FormDescription>
        <FormInputAction>
          <Input
            defaultValue="${rootDir}/qa-report.html"
            className="flex-1 font-mono text-xs h-8"
          />
          <Button variant="outline" size="sm" className="h-8 text-xs shrink-0">
            Browse
          </Button>
        </FormInputAction>
      </FormItem>
    </div>
  ),
};

export const InputWithMultipleActions: StoryObj = {
  name: "FormInputAction — Multiple Actions",
  render: () => (
    <div className="max-w-sm">
      <FormItem>
        <FormLabel>TM File</FormLabel>
        <FormInputAction>
          <Input
            defaultValue="/data/project.tmx"
            className="flex-1 font-mono text-xs h-8"
          />
          <Button variant="outline" size="sm" className="h-8 text-xs shrink-0">
            Browse
          </Button>
          <Button variant="ghost" size="sm" className="h-8 text-xs shrink-0">
            Clear
          </Button>
        </FormInputAction>
      </FormItem>
    </div>
  ),
};

// ── FormFieldGroup ───────────────────────────────────────────────────

export const FieldGroup: StoryObj = {
  name: "FormFieldGroup — Non-Collapsible",
  render: () => (
    <div className="max-w-xs">
      <FormFieldGroup label="General Settings">
        <div className="space-y-2">
          <FormItem>
            <FormLabel>Name</FormLabel>
            <FormControl><Input defaultValue="My Project" /></FormControl>
          </FormItem>
          <FormItem>
            <FormLabel>Description</FormLabel>
            <FormControl><Input placeholder="Optional description..." /></FormControl>
          </FormItem>
        </div>
      </FormFieldGroup>
    </div>
  ),
};

export const CollapsibleGroup: StoryObj = {
  name: "FormFieldGroup — Collapsible",
  render: () => (
    <div className="max-w-xs space-y-4">
      <FormFieldGroup label="Whitespace Checks" collapsible>
        <div className="space-y-1">
          <FormToggle checked={true} onCheckedChange={() => {}} label="Leading whitespace" compact />
          <FormToggle checked={true} onCheckedChange={() => {}} label="Trailing whitespace" compact />
          <FormToggle checked={false} onCheckedChange={() => {}} label="Double spaces" compact />
        </div>
      </FormFieldGroup>

      <FormFieldGroup label="Content Checks" collapsible defaultCollapsed>
        <div className="space-y-1">
          <FormToggle checked={true} onCheckedChange={() => {}} label="Empty target" compact />
          <FormToggle checked={true} onCheckedChange={() => {}} label="Target same as source" compact />
        </div>
      </FormFieldGroup>

      <FormFieldGroup label="Output" collapsible defaultCollapsed>
        <div className="space-y-2">
          <FormItem>
            <FormLabel>Report Path</FormLabel>
            <FormInputAction>
              <Input defaultValue="${rootDir}/qa-report.html" className="flex-1 font-mono text-xs h-8" />
              <Button variant="outline" size="sm" className="h-8 text-xs shrink-0">Browse</Button>
            </FormInputAction>
          </FormItem>
        </div>
      </FormFieldGroup>
    </div>
  ),
};

// ── Composed Example ─────────────────────────────────────────────────

export const ComposedConfigEditor: StoryObj = {
  name: "Composed — Mini Config Editor",
  render: () => {
    const [useTM, setUseTM] = useState(false);
    const [threshold, setThreshold] = useState("95");
    const [format, setFormat] = useState("html");

    return (
      <div className="max-w-sm space-y-4">
        <FormFieldGroup label="Translation Memory">
          <div className="space-y-2">
            <FormToggle
              checked={useTM}
              onCheckedChange={setUseTM}
              label="Use Translation Memory"
              description="Leverage existing translations for pre-population."
            />

            <FormItem disabled={!useTM}>
              <FormLabel disabled={!useTM}>TM File</FormLabel>
              <FormInputAction>
                <Input
                  defaultValue="/data/project.tmx"
                  disabled={!useTM}
                  className="flex-1 font-mono text-xs h-8"
                />
                <Button variant="outline" size="sm" disabled={!useTM} className="h-8 text-xs shrink-0">
                  Browse
                </Button>
              </FormInputAction>
            </FormItem>

            <FormItem disabled={!useTM}>
              <FormLabel disabled={!useTM}>Match Threshold</FormLabel>
              <FormDescription>Minimum similarity score (0–100).</FormDescription>
              <FormControl>
                <Input
                  type="number"
                  min={0}
                  max={100}
                  value={threshold}
                  disabled={!useTM}
                  className="h-8 text-xs"
                  onChange={(e) => setThreshold(e.target.value)}
                />
              </FormControl>
            </FormItem>
          </div>
        </FormFieldGroup>

        <FormFieldGroup label="Output" collapsible>
          <div className="space-y-2">
            <FormItem>
              <FormLabel>Report Format</FormLabel>
              <FormControl>
                <Select value={format} onValueChange={setFormat}>
                  <SelectTrigger className="h-8 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="html">HTML file</SelectItem>
                    <SelectItem value="tsv">Tab-delimited file</SelectItem>
                    <SelectItem value="xml">XML file</SelectItem>
                  </SelectContent>
                </Select>
              </FormControl>
            </FormItem>
          </div>
        </FormFieldGroup>
      </div>
    );
  },
};
