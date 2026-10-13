import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { LocaleList } from "@/components/LocalePicker";

describe("<LocaleList>", () => {
  it("lists every supported language without a search box", async () => {
    const onChange = vi.fn();
    render(<LocaleList value="en" onChange={onChange} />);

    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(await screen.findAllByRole("option")).toHaveLength(3);

    fireEvent.click(screen.getByRole("option", { name: /Malaysia/ }));
    expect(onChange).toHaveBeenCalledWith("ms");
  });
});
