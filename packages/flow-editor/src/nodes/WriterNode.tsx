import { Position } from "@xyflow/react";
import { FileOutput } from "lucide-react";
import { createTerminalNode } from "./TerminalNode";

export const WriterNode = createTerminalNode({
  accent: "oklch(0.65 0.19 252)",
  icon: FileOutput,
  typeLabel: "Output",
  defaultLabel: "Writer",
  handleType: "target",
  handlePosition: Position.Left,
});
