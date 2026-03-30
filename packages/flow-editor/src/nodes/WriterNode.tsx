import { Position } from "@xyflow/react";
import { FileOutput } from "lucide-react";
import { createTerminalNode } from "./TerminalNode";

export const WriterNode = createTerminalNode({
  accent: "oklch(0.7 0.13 85)",
  icon: FileOutput,
  typeLabel: "Output",
  defaultLabel: "Writer",
  handleType: "target",
  handlePosition: Position.Left,
});
