import fs from "fs";
import path from "path";

const dir = import.meta.dirname ?? __dirname;
const OUTPUT_DIR = path.resolve(dir, "output");
const PUBLISH_DIR = path.resolve(dir, "..", "static", "video", "polished");

fs.mkdirSync(PUBLISH_DIR, { recursive: true });

const files = fs.readdirSync(OUTPUT_DIR).filter((f) => f.endsWith(".mp4"));

if (files.length === 0) {
  console.error("No MP4 files found in output/. Run 'npm run build' first.");
  process.exit(1);
}

for (const file of files) {
  const src = path.join(OUTPUT_DIR, file);
  const dst = path.join(PUBLISH_DIR, file);
  fs.copyFileSync(src, dst);
  console.log(`  ${file} -> ${dst}`);
}

console.log(`\nPublished ${files.length} video(s) to ${PUBLISH_DIR}`);
