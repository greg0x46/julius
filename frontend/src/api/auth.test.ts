import { expect, it } from "vitest";
import { AuthError, authErrorMessage } from "./auth";

it("localizes known authentication problems and preserves Portuguese details", () => {
  const error = new AuthError("E-mail ou senha incorretos, ou sessão expirada.", "/problems/unauthorized");
  expect(authErrorMessage(error, "pt-BR", "fallback")).toBe(error.message);
  expect(authErrorMessage(error, "en", "fallback")).toBe("Incorrect email or password, or session expired.");
  expect(authErrorMessage(new AuthError("erro", "/problems/unknown"), "en", "fallback")).toBe("Could not complete authentication.");
});
