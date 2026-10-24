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

// jsdom 30 added the -webkit- animation and transition properties that jsdom 27
// lacked. rc-motion (antd) detects CSS motion support from them, so antd would
// start animating, and jsdom never fires animationend/transitionend: leaving
// elements (a button's loading icon, a closed dropdown) would stay mounted
// and keep `pointer-events: none`. Keep motion off, as it was before.
// Must run before antd is imported, because rc-motion decides once at load.
for (const property of ["WebkitAnimation", "WebkitTransition"]) {
  Reflect.deleteProperty(Object.getPrototypeOf(document.createElement("div").style), property);
}

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
