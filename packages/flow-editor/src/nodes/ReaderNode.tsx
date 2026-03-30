import { Position } from "@xyflow/react";
import { FileInput } from "lucide-react";
import { createTerminalNode } from "./TerminalNode";

export const ReaderNode = createTerminalNode({
  accent: "oklch(0.7 0.17 145)",
  icon: FileInput,
  typeLabel: "Input",
  defaultLabel: "Reader",
  handleType: "source",
  handlePosition: Position.Right,
});
