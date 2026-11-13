import type { Metadata } from "next"

export const metadata: Metadata = {
  title: "Terms of Service - Vecrow",
}

export default function Page() {
  return (
    <>
      <div>
        <h1>Terms of Service</h1>
        <p className="mt-1">Draft. These terms will change before launch.</p>
      </div>
      <section>
        <h2>The service</h2>
        <p>
          Vecrow is a hackathon project under active development. It holds USDC
          for freelance work in a smart contract on NEAR and releases it per
          milestone. Use it for testing only, with funds you can afford to lose.
        </p>
      </section>
      <section>
        <h2>Your wallet, your keys</h2>
        <p>
          You sign in and approve actions with your own wallet. Vecrow never
          holds your keys. Funds leave an escrow only through the
          contract&apos;s methods. A lost wallet cannot be recovered by Vecrow.
        </p>
      </section>
      <section>
        <h2>No warranty</h2>
        <p>
          The service is provided as is, without any warranty. Vecrow does not
          arbitrate disputes between a client and a freelancer.
        </p>
      </section>
    </>
  )
}
