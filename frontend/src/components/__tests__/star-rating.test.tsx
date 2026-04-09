import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { StarRating } from "@/components/star-rating";

describe("StarRating", () => {
  it("renders correct number of stars", () => {
    const { container } = render(<StarRating rating={3} />);
    const stars = container.querySelectorAll("svg");
    expect(stars).toHaveLength(5);
  });

  it("renders custom max stars", () => {
    const { container } = render(<StarRating rating={2} max={10} />);
    const stars = container.querySelectorAll("svg");
    expect(stars).toHaveLength(10);
  });

  it("shows rating value when showValue is true", () => {
    render(<StarRating rating={4.5} showValue />);
    expect(screen.getByText("4.5")).toBeInTheDocument();
  });

  it("does not show rating value by default", () => {
    render(<StarRating rating={4.5} />);
    expect(screen.queryByText("4.5")).not.toBeInTheDocument();
  });

  it("fills correct number of stars based on rating", () => {
    const { container } = render(<StarRating rating={3} />);
    const filled = container.querySelectorAll(".fill-amber-500");
    expect(filled).toHaveLength(3);
  });

  it("calls onChange when interactive star is clicked", () => {
    const onChange = vi.fn();
    const { container } = render(
      <StarRating rating={2} interactive onChange={onChange} />,
    );
    const stars = container.querySelectorAll("svg");
    fireEvent.click(stars[3]);
    expect(onChange).toHaveBeenCalledWith(4);
  });

  it("does not call onChange when not interactive", () => {
    const onChange = vi.fn();
    const { container } = render(
      <StarRating rating={2} onChange={onChange} />,
    );
    const stars = container.querySelectorAll("svg");
    fireEvent.click(stars[3]);
    expect(onChange).not.toHaveBeenCalled();
  });
});
