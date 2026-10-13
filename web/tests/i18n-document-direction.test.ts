import { afterEach, describe, expect, it } from "vitest";
import i18n from "@/i18n";
import { loadLocale } from "@/utils/i18n";

const originalLanguage = i18n.language;
const originalLangAttribute = document.documentElement.getAttribute("lang");
const originalDirAttribute = document.documentElement.getAttribute("dir");

const restoreAttribute = (name: "lang" | "dir", value: string | null) => {
  if (value === null) {
    document.documentElement.removeAttribute(name);
  } else {
    document.documentElement.setAttribute(name, value);
  }
};

describe("document locale direction", () => {
  afterEach(async () => {
    localStorage.clear();
    restoreAttribute("lang", originalLangAttribute);
    restoreAttribute("dir", originalDirAttribute);
    await i18n.changeLanguage(originalLanguage);
  });

  it.each([
    ["en", "ltr"],
    ["id", "ltr"],
    ["ms", "ltr"],
  ] as const)("sets %s as a %s document", (locale, direction) => {
    loadLocale(locale);

    expect(document.documentElement).toHaveAttribute("lang", locale);
    expect(document.documentElement).toHaveAttribute("dir", direction);
  });
});
