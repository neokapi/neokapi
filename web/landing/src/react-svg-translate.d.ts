// React's SVG attribute types omit the W3C ITS `translate` flag, but
// @neokapi/kapi-react honors it when deciding what to extract — allow it on
// SVG elements (used on <text> labels in the pipeline diagram). The import
// makes this file a module so the declaration augments React instead of
// replacing it.
import "react";

declare module "react" {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  interface SVGAttributes<T> {
    translate?: "yes" | "no";
  }
}
