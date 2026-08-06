import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { LocationField, type LocationIntent } from "@/features/profile/components/LocationField"

const PERMISSION_DENIED = 1

function stubGeolocation(impl: Partial<Geolocation>) {
  vi.stubGlobal("navigator", { ...globalThis.navigator, geolocation: impl as Geolocation })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("LocationField", () => {
  it("reports captured coordinates when the browser grants permission", async () => {
    stubGeolocation({
      getCurrentPosition: (success) => {
        ;(success as PositionCallback)({
          coords: { latitude: 40.4168, longitude: -3.7038 },
        } as GeolocationPosition)
      },
    })
    const onChange = vi.fn()
    const user = userEvent.setup()

    render(<LocationField hasStoredLocation={false} intent={{ kind: "unchanged" }} onChange={onChange} />)
    await user.click(screen.getByRole("button", { name: /usar mi ubicación/i }))

    expect(onChange).toHaveBeenCalledWith({ kind: "captured", latitude: 40.4168, longitude: -3.7038 })
  })

  it("explains the denial instead of failing when permission is refused", async () => {
    stubGeolocation({
      getCurrentPosition: (_success, error) => {
        ;(error as PositionErrorCallback)({
          code: PERMISSION_DENIED,
          PERMISSION_DENIED,
        } as GeolocationPositionError)
      },
    })
    const onChange = vi.fn()
    const user = userEvent.setup()

    render(<LocationField hasStoredLocation={false} intent={{ kind: "unchanged" }} onChange={onChange} />)
    await user.click(screen.getByRole("button", { name: /usar mi ubicación/i }))

    expect(await screen.findByText(/no diste permiso de ubicación/i)).toBeInTheDocument()
    expect(onChange).not.toHaveBeenCalled()
  })

  it("offers to stop sharing only when a location is active", async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()

    const { rerender } = render(
      <LocationField hasStoredLocation={false} intent={{ kind: "unchanged" }} onChange={onChange} />,
    )
    expect(screen.queryByRole("button", { name: /dejar de compartir/i })).not.toBeInTheDocument()

    rerender(<LocationField hasStoredLocation intent={{ kind: "unchanged" }} onChange={onChange} />)
    await user.click(screen.getByRole("button", { name: /dejar de compartir/i }))

    expect(onChange).toHaveBeenCalledWith({ kind: "cleared" })
  })

  it("lets the user undo a pending removal", async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    const intent: LocationIntent = { kind: "cleared" }

    render(<LocationField hasStoredLocation intent={intent} onChange={onChange} />)
    expect(screen.getByText(/se dejará de compartir al guardar/i)).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: /deshacer/i }))
    expect(onChange).toHaveBeenCalledWith({ kind: "unchanged" })
  })

  it("reports gracefully when the browser has no geolocation at all", async () => {
    vi.stubGlobal("navigator", { ...globalThis.navigator, geolocation: undefined })
    const user = userEvent.setup()

    render(<LocationField hasStoredLocation={false} intent={{ kind: "unchanged" }} onChange={vi.fn()} />)
    await user.click(screen.getByRole("button", { name: /usar mi ubicación/i }))

    expect(await screen.findByText(/no puede darnos tu ubicación/i)).toBeInTheDocument()
  })
})
