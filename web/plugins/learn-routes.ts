/**
 * One page per learning lab.
 *
 * The curriculum (packages/kapi-learn/src/curriculum) is data: a list of labs
 * with stable ids. This plugin gives each its own route, /learn/<id>, served
 * by the LabPage component with the id as a prop, so a lab has a page of its
 * own to link to, with a title and a description the crawler and a shared
 * link preview can read. The /learn index is an ordinary page under src/pages.
 *
 * Loaded through Docusaurus's TypeScript plugin support, so it imports the
 * curriculum's source directly; a lab added to the curriculum is a page on the
 * next build, with nothing to register here.
 */

import type { LoadContext, Plugin } from "@docusaurus/types";
import { LABS } from "@neokapi/kapi-learn/curriculum";

export default function learnRoutesPlugin(_context: LoadContext): Plugin<undefined> {
  return {
    name: "neokapi-learn-routes",

    async contentLoaded({ actions }) {
      for (const lab of LABS) {
        actions.addRoute({
          path: `/learn/${lab.id}`,
          component: "@site/src/components/Learn/LabPage",
          exact: true,
          props: { labId: lab.id },
        });
      }
    },
  };
}
