import BrowserOnly from "@docusaurus/BrowserOnly";

interface ThemedVideoProps {
  sources: {
    light: string;
    dark: string;
  };
  maxWidth?: string;
}

function ThemedVideoInner({ sources, maxWidth = "800px" }: ThemedVideoProps) {
  // Lazy-import inside BrowserOnly so the hook only runs in the browser
  // (Docusaurus context isn't available during SSG for node_modules consumers).
  const { useColorMode } = require("@docusaurus/theme-common") as {
    useColorMode: () => { colorMode: "light" | "dark" };
  };
  const { colorMode } = useColorMode();
  const src = colorMode === "dark" ? sources.dark : sources.light;

  return (
    <video controls width="100%" style={{ maxWidth }} key={src}>
      <source src={src} type="video/webm" />
      Your browser does not support the video tag.
    </video>
  );
}

export default function ThemedVideo(props: ThemedVideoProps) {
  return (
    <BrowserOnly fallback={
      <video controls width="100%" style={{ maxWidth: props.maxWidth ?? "800px" }}>
        <source src={props.sources.light} type="video/webm" />
      </video>
    }>
      {() => <ThemedVideoInner {...props} />}
    </BrowserOnly>
  );
}
