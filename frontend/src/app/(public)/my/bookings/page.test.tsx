import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"
import MyBookingsPage from "./page"

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

vi.mock("@/store/auth", () => ({
  useAuthStore: () => ({ token: "token", hydrated: true }),
}))

const getMyBookings = vi.fn()
const getMyClientMasterBookings = vi.fn()

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api")
  return {
    ...actual,
    getMyBookings: (...args: unknown[]) => getMyBookings(...args),
    getMyClientMasterBookings: (...args: unknown[]) =>
      getMyClientMasterBookings(...args),
    cancelBooking: vi.fn(),
  }
})

vi.mock("@/components/banya/booking-chat-panel", () => ({
  BookingChatPanel: () => <div data-testid="booking-chat-panel" />,
}))

describe("MyBookingsPage", () => {
  beforeEach(() => {
    getMyBookings.mockResolvedValue([])
    getMyClientMasterBookings.mockResolvedValue({
      bookings: [
        {
          id: "mb-1",
          master_id: "master-1",
          client_user_id: "user-1",
          date: "2026-05-07",
          time_from: "12:00",
          time_to: "13:00",
          comment: "",
          status: "confirmed",
          created_at: "2026-05-05T10:00:00Z",
        },
      ],
    })
  })

  it("shows master bookings in upcoming tab", async () => {
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={qc}>
        <MyBookingsPage />
      </QueryClientProvider>,
    )

    await waitFor(() => {
      expect(screen.getByText("Пар-мастер")).toBeInTheDocument()
      expect(screen.getByText("Чат по брони")).toBeInTheDocument()
    })
  })
})

