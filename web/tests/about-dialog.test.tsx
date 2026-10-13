import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import AboutDialog from "@/components/AboutDialog";

vi.mock("@/contexts/InstanceContext", () => ({ useInstance: () => ({ profile: { version: "26.10" } }) }));
vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string, params?: Record<string, unknown>) => (params ? `${key} ${JSON.stringify(params)}` : key),
}));

describe("<AboutDialog>", () => {
  it("credits Cloud Creatures with a copyright line and a link to the studio", () => {
    render(<AboutDialog open onOpenChange={vi.fn()} />);

    expect(screen.getByRole("heading", { name: "Roster Tracker" })).toBeInTheDocument();
    expect(screen.getByText(/about\.by .*Cloud Creatures/)).toBeInTheDocument();
    expect(screen.getByText(new RegExp(`about\\.copyright .*"year":${new Date().getFullYear()}`))).toBeInTheDocument();
    const link = screen.getByRole("link", { name: /thecloudcreatures\.com/ });
    expect(link).toHaveAttribute("href", "https://thecloudcreatures.com/");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noreferrer");
  });
});
