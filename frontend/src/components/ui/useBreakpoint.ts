import { useEffect, useState } from "react";
export function useBreakpoint() {
  const read = () => ({
    xs: matchMedia("(max-width: 575px)").matches,
    sm: matchMedia("(min-width: 576px)").matches,
    md: matchMedia("(min-width: 768px)").matches,
    lg: matchMedia("(min-width: 992px)").matches,
    xl: matchMedia("(min-width: 1200px)").matches,
    xxl: matchMedia("(min-width: 1600px)").matches,
  });
  const [screens, setScreens] = useState(read);
  useEffect(() => {
    const queries = [
      "(max-width: 575px)",
      ...[576, 768, 992, 1200, 1600].map((width) => `(min-width: ${width}px)`),
    ].map((query) => matchMedia(query));
    const update = () => setScreens(read());
    queries.forEach((query) => query.addEventListener("change", update));
    return () =>
      queries.forEach((query) => query.removeEventListener("change", update));
  }, []);
  return screens;
}
