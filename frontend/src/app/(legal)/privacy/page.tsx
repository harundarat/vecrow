import type { Metadata } from "next"

export const metadata: Metadata = {
  title: "Privacy Policy - Vecrow",
}

export default function Page() {
  return (
    <>
      <div>
        <h1>Privacy Policy</h1>
        <p className="mt-1">Draft. This policy will change before launch.</p>
      </div>
      <section>
        <h2>What we store</h2>
        <ul>
          <li>Your username and the wallet addresses you link to it.</li>
          <li>An email address, only if you add and verify one.</li>
          <li>
            The escrows you create or join, including the addresses that sign
            them.
          </li>
        </ul>
      </section>
      <section>
        <h2>Sessions</h2>
        <p>
          When you sign in, your browser keeps a session token. The server
          stores only a hash of it, and the session expires on its own or when
          you log out.
        </p>
      </section>
      <section>
        <h2>On-chain data</h2>
        <p>
          Escrows, signatures and payouts are recorded on public blockchains.
          Anyone can read them, and they cannot be deleted.
        </p>
      </section>
    </>
  )
}
