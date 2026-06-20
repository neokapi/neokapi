import React from "react";
import { Redirect } from "@docusaurus/router";

// This lab was consolidated (see the Labs menu). Redirect old links to its new
// home so bookmarks and external references keep working.
export default function RedirectPage(): React.ReactElement {
  return <Redirect to="/lab/structure" />;
}
