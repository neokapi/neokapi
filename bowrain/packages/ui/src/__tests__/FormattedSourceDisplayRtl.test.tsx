import { describe, it, expect } from "vite-plus/test";
import { render } from "@testing-library/react";

import { FormattedSourceDisplay } from "../components/editor/FormattedSourceDisplay";

describe("FormattedSourceDisplay — writing direction", () => {
  it("puts dir/lang on its own root element for an RTL locale", () => {
    const { container } = render(
      <FormattedSourceDisplay codedText="مرحباً بالعالم" spans={[]} locale="ar-EG" />,
    );
    const root = container.firstElementChild;
    expect(root?.getAttribute("dir")).toBe("rtl");
    expect(root?.getAttribute("lang")).toBe("ar-EG");
    expect(root?.textContent).toContain("مرحباً");
  });

  it("defaults to ltr with no lang asserted when no locale is given", () => {
    const { container } = render(<FormattedSourceDisplay codedText="hello" spans={[]} />);
    const root = container.firstElementChild;
    expect(root?.getAttribute("dir")).toBe("ltr");
    expect(root?.getAttribute("lang")).toBeNull();
  });
});
