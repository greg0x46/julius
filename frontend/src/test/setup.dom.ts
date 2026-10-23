// Setup for the jsdom ("dom") Vitest project only.
import "@testing-library/jest-dom/vitest";
import { cleanup, configure } from "@testing-library/react";
import { afterEach, vi } from "vitest";

// when several workers share the CPU.
configure({ asyncUtilTimeout: 5000 });

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

// jsdom does not implement the browser's dialog lifecycle.
HTMLDialogElement.prototype.showModal = function () { this.setAttribute("open", ""); };
HTMLDialogElement.prototype.close = function () { this.removeAttribute("open"); };

// jsdom recognizes the popover attribute but has no top-layer implementation.
HTMLElement.prototype.showPopover = function () { this.style.display = "block"; };
HTMLElement.prototype.hidePopover = function () { this.style.display = "none"; };
