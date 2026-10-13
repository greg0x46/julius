import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { BuildFooter } from "./BuildFooter";

const sha = "a1b2c3d000000000000000000000000000000000";

describe("BuildFooter", () => {
  it.each(["v0.4.2", "stage", "dev"])("shows %s and links to the exact commit", (version) => {
    render(<BuildFooter version={version} commit={sha} />);
    expect(screen.getByRole("contentinfo")).toHaveTextContent(`Julius ${version} · a1b2c3d`);
    expect(screen.getByRole("link", { name: "Ver commit a1b2c3d no GitHub" })).toHaveAttribute(
      "href", `https://github.com/greg0x46/julius/commit/${sha}`,
    );
  });

  it.each(["", "unknown", "a1b2c3d", "../../invalid"])("omits unavailable or invalid commit %s", (commit) => {
    render(<BuildFooter version="stage" commit={commit} />);
    expect(screen.getByRole("contentinfo")).toHaveTextContent(/^Julius stage$/);
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  it("falls back to dev without version metadata", () => {
    render(<BuildFooter version="  " commit="" />);
    expect(screen.getByRole("contentinfo")).toHaveTextContent(/^Julius dev$/);
  });

  it("uses baked-in metadata and keeps it on rerender", () => {
    vi.stubEnv("VITE_APP_VERSION", "v9.8.7");
    vi.stubEnv("VITE_APP_COMMIT", sha);
    try {
      const { rerender } = render(<BuildFooter />);
      rerender(<BuildFooter />);
      expect(screen.getByRole("contentinfo")).toHaveTextContent("Julius v9.8.7 · a1b2c3d");
    } finally {
      vi.unstubAllEnvs();
    }
  });
});
