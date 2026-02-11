import { useColorMode } from "@docusaurus/theme-common";

interface ThemedVideoProps {
  sources: {
    light: string;
    dark: string;
  };
  maxWidth?: string;
}

export default function ThemedVideo({ sources, maxWidth = "800px" }: ThemedVideoProps) {
  const { colorMode } = useColorMode();
  const src = colorMode === "dark" ? sources.dark : sources.light;

  return (
    <video controls width="100%" style={{ maxWidth }} key={src}>
      <source src={src} type="video/webm" />
      Your browser does not support the video tag.
    </video>
  );
}
