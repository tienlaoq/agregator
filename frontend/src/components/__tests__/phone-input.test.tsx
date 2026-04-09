import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { PhoneInput, getRawPhone } from "@/components/banya/phone-input";

describe("PhoneInput", () => {
  it("renders with placeholder", () => {
    render(<PhoneInput value="" onChange={() => {}} />);
    expect(
      screen.getByPlaceholderText("+7 (999) 123-45-67"),
    ).toBeInTheDocument();
  });

  it("displays formatted value", () => {
    render(<PhoneInput value="+7 (999) 123-45-67" onChange={() => {}} />);
    const input = screen.getByDisplayValue("+7 (999) 123-45-67");
    expect(input).toBeInTheDocument();
  });

  it("calls onChange with formatted phone on input", () => {
    const onChange = vi.fn();
    render(<PhoneInput value="" onChange={onChange} />);
    const input = screen.getByPlaceholderText("+7 (999) 123-45-67");

    fireEvent.change(input, { target: { value: "79991234567" } });

    expect(onChange).toHaveBeenCalled();
    const formatted = onChange.mock.calls[0][0];
    expect(formatted).toContain("+7");
  });

  it("sets +7 ( on focus when empty", () => {
    const onChange = vi.fn();
    render(<PhoneInput value="" onChange={onChange} />);
    const input = screen.getByPlaceholderText("+7 (999) 123-45-67");
    fireEvent.focus(input);
    expect(onChange).toHaveBeenCalledWith("+7 (");
  });

  it("clears value on blur if only prefix", () => {
    const onChange = vi.fn();
    render(<PhoneInput value="+7 (" onChange={onChange} />);
    const input = screen.getByDisplayValue("+7 (");
    fireEvent.blur(input);
    expect(onChange).toHaveBeenCalledWith("");
  });
});

describe("getRawPhone", () => {
  it("extracts raw phone from formatted string", () => {
    expect(getRawPhone("+7 (999) 123-45-67")).toBe("+79991234567");
  });

  it("returns original value if too short", () => {
    expect(getRawPhone("+7 (99")).toBe("+7 (99");
  });
});
