import type {
  BravoConversation,
  BravoMessage,
  BravoToolCall,
  BravoConfig,
  BravoToolInfo,
  BravoUsageSummary,
} from "../../types/api";

export const sampleConversations: BravoConversation[] = [
  {
    id: "conv-1",
    workspace_id: "ws-1",
    user_id: "user-1",
    project_id: "proj-1",
    title: "Translate French files",
    status: "active",
    created_at: new Date(Date.now() - 3600000).toISOString(),
    updated_at: new Date(Date.now() - 600000).toISOString(),
  },
  {
    id: "conv-2",
    workspace_id: "ws-1",
    user_id: "user-1",
    project_id: "proj-1",
    title: "Review QA issues",
    status: "completed",
    created_at: new Date(Date.now() - 86400000).toISOString(),
    updated_at: new Date(Date.now() - 86400000).toISOString(),
  },
  {
    id: "conv-3",
    workspace_id: "ws-1",
    user_id: "user-1",
    project_id: "",
    title: "Help me pseudo-translate",
    status: "failed",
    created_at: new Date(Date.now() - 172800000).toISOString(),
    updated_at: new Date(Date.now() - 172800000).toISOString(),
  },
];

export const sampleToolCall: BravoToolCall = {
  id: "tc-1",
  message_id: "msg-2",
  tool_name: "run_flow",
  input: { flow: "pseudo-translate", target_lang: "qps" },
  output: { blocks_processed: 42, blocks_skipped: 3 },
  status: "completed",
  duration: 1250000000, // 1.25s in nanoseconds
  error: "",
};

export const sampleApprovalToolCall: BravoToolCall = {
  id: "tc-2",
  message_id: "msg-3",
  tool_name: "connector_push",
  input: { connector_id: "git-main", project_id: "proj-1" },
  status: "needs_approval",
  duration: 0,
};

export const sampleMessages: BravoMessage[] = [
  {
    id: "msg-1",
    conversation_id: "conv-1",
    role: "user",
    content: "Can you pseudo-translate the French files?",
    created_at: new Date(Date.now() - 300000).toISOString(),
  },
  {
    id: "msg-2",
    conversation_id: "conv-1",
    role: "assistant",
    content:
      'Sure! I\'ll run the pseudo-translate flow on the French target files.\n\nHere\'s a quick script to verify the output:\n```python\nimport json\nwith open("fr-FR.json") as f:\n    data = json.load(f)\nprint(f"Keys: {len(data)}")\n```',
    tool_calls: [sampleToolCall],
    input_tokens: 1500,
    output_tokens: 480,
    created_at: new Date(Date.now() - 240000).toISOString(),
  },
  {
    id: "msg-3",
    conversation_id: "conv-1",
    role: "assistant",
    content: "Done! 42 blocks were pseudo-translated. I'd like to push the results to git — shall I?",
    tool_calls: [sampleApprovalToolCall],
    input_tokens: 800,
    output_tokens: 120,
    created_at: new Date(Date.now() - 180000).toISOString(),
  },
];

export const sampleConfig: BravoConfig = {
  workspace_id: "ws-1",
  enabled: true,
  allowed_tools: [],
  denied_tools: ["execute_script"],
  require_approval: ["connector_push", "connector_pull"],
  code_exec_enabled: false,
  max_concurrent: 3,
};

export const sampleTools: BravoToolInfo[] = [
  { name: "list_projects", require_approval: false },
  { name: "get_project", require_approval: false },
  { name: "list_blocks", require_approval: false },
  { name: "run_flow", require_approval: false },
  { name: "tm_search", require_approval: false },
  { name: "connector_push", require_approval: true },
  { name: "connector_pull", require_approval: true },
];

export const sampleUsage: BravoUsageSummary = {
  workspace_id: "ws-1",
  total_input_tokens: 45200,
  total_output_tokens: 12800,
  total_container_sec: 3720,
  message_count: 38,
};
