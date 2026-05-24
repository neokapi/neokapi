import Layout from "@theme/Layout";
import CuratedDemo from "@site/src/components/curated/CuratedDemo";

// Scratch route (/curated-demo) for the R8 curated result-view components.
// Thin wrapper — all the demo content lives in src/components/curated/.
export default function CuratedDemoPage() {
  return (
    <Layout
      title="Curated result views (demo)"
      description="Demo of the framework-first curated result-view components."
    >
      <CuratedDemo />
    </Layout>
  );
}
