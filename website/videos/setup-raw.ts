import fs from "fs";
import path from "path";

const dir = import.meta.dirname ?? __dirname;
const publicRaw = path.resolve(dir, "public", "raw");
const staticVideo = path.resolve(dir, "..", "static", "video");

const links: [string, string][] = [
  ["bowrain/dark", path.join(staticVideo, "bowrain", "dark")],
  ["bowrain/light", path.join(staticVideo, "bowrain", "light")],
  ["kapi", path.join(staticVideo, "kapi")],
  ["bowrain-cli", path.join(staticVideo, "bowrain-cli")],
  ["web-app/dark", path.join(staticVideo, "web-app", "dark")],
  ["web-app/light", path.join(staticVideo, "web-app", "light")],
];

fs.mkdirSync(publicRaw, { recursive: true });

for (const [name, target] of links) {
  const linkPath = path.join(publicRaw, name);
  const linkDir = path.dirname(linkPath);
  fs.mkdirSync(linkDir, { recursive: true });

  // Remove existing symlink if present
  try {
    const stat = fs.lstatSync(linkPath);
    if (stat.isSymbolicLink() || stat.isDirectory()) {
      fs.rmSync(linkPath, { recursive: true });
    }
  } catch {
    // doesn't exist, fine
  }

  if (fs.existsSync(target)) {
    fs.symlinkSync(target, linkPath, "dir");
    console.log(`  ${name} -> ${target}`);
  } else {
    console.log(`  ${name} (skipped, target not found: ${target})`);
  }
}

console.log("Raw recording symlinks set up.");
