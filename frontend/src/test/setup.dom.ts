// Setup for the jsdom ("dom") Vitest project only.
import "@testing-library/jest-dom/vitest";
import { cleanup, configure } from "@testing-library/react";
import { afterEach, vi } from "vitest";

// The default 1 s findBy*/waitFor timeout is too tight for antd renders
// when several workers share the CPU.
configure({ asyncUtilTimeout: 5000 });

// jsdom has no viewport. Answer antd's breakpoint queries as a desktop
// would (every min-width matches, no max-width does) so components that
// switch layout on Grid.useBreakpoint render their default, wide form;
// a test that needs the compact form overrides this itself.
Object.defineProperty(window, "matchMedia", {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: /min-width/.test(query),
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
});

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

globalThis.ResizeObserver = ResizeObserverStub;

afterEach(() => {
  cleanup();
});
