import { describe, expect, it } from "vitest";
import { locales } from "@/i18n";
import enTranslation from "@/locales/en.json";
import idTranslation from "@/locales/id.json";
import msTranslation from "@/locales/ms.json";
import { getLocaleDisplayName } from "@/utils/i18n";

const flattenTranslationKeys = (value: unknown, prefix = ""): string[] => {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    return Object.entries(value).flatMap(([key, child]) => flattenTranslationKeys(child, prefix ? `${prefix}.${key}` : key));
  }

  return [prefix];
};

describe("supported locales", () => {
  it("supports only English, Indonesian, and Malay", () => {
    expect([...locales].sort()).toEqual(["en", "id", "ms"]);
  });

  it("labels the languages without the word Bahasa", () => {
    expect(getLocaleDisplayName("en")).toBe("English");
    expect(getLocaleDisplayName("id")).toBe("Indonesia");
    expect(getLocaleDisplayName("ms")).toBe("Malaysia");
  });

  it("keeps the Indonesian and Malay catalogs aligned with the English one", () => {
    const english = flattenTranslationKeys(enTranslation).sort();
    expect(flattenTranslationKeys(idTranslation).sort()).toEqual(english);
    expect(flattenTranslationKeys(msTranslation).sort()).toEqual(english);
  });
});
