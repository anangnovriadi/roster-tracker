import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import MemosLogo from "@/components/MemosLogo";

const fullLogoUrl = "/logo.svg";

const mockInstance = {
  generalSetting: {} as { customProfile?: { title?: string; logoUrl?: string } },
};

vi.mock("@/contexts/InstanceContext", () => ({
  useInstance: () => mockInstance,
}));

describe("<MemosLogo>", () => {
  beforeEach(() => {
    mockInstance.generalSetting = {};
  });

  it("uses the bundled logo when the instance has no custom logo", () => {
    const { container } = render(<MemosLogo />);

    expect(container.querySelector("img")).toHaveAttribute("src", fullLogoUrl);
    expect(screen.getByText("Roster Tracker")).toBeInTheDocument();
  });

  it("uses custom instance branding when configured", () => {
    mockInstance.generalSetting = { customProfile: { title: "Team Notes", logoUrl: "/custom-logo.png" } };
    const { container } = render(<MemosLogo />);

    expect(container.querySelector("img")).toHaveAttribute("src", "/custom-logo.png");
    expect(screen.getByText("Team Notes")).toBeInTheDocument();
  });
});
