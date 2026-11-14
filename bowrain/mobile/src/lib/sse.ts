/**
 * Splits a server-sent event stream into the data of its `change` events.
 * Feed it text as it arrives; it returns the complete events and keeps the
 * unfinished rest.
 */
export function createSseParser(): (chunk: string) => string[] {
  let buffer = "";
  return (chunk) => {
    buffer += chunk.replace(/\r\n/g, "\n");
    const out: string[] = [];
    let end = buffer.indexOf("\n\n");
    while (end >= 0) {
      const frame = buffer.slice(0, end);
      buffer = buffer.slice(end + 2);
      let event = "message";
      const data: string[] = [];
      for (const line of frame.split("\n")) {
        if (line.startsWith(":")) continue;
        if (line.startsWith("event:")) event = line.slice(6).trim();
        else if (line.startsWith("data:")) data.push(line.slice(5).trimStart());
      }
      if (event === "change" && data.length > 0) out.push(data.join("\n"));
      end = buffer.indexOf("\n\n");
    }
    return out;
  };
}

/** The type of a relayed change event, or "" for a frame that does not parse. */
export function changeTypeOf(data: string): string {
  try {
    const ev = JSON.parse(data) as { type?: unknown };
    return typeof ev.type === "string" ? ev.type : "";
  } catch {
    return "";
  }
}
