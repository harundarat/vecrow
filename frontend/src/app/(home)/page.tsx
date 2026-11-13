"use client"
import { Button } from "@/components/ui/button";
import type { Provider } from "@reown/appkit-adapter-solana";
import { useAppKit, useAppKitAccount, useAppKitProvider } from "@reown/appkit/react";
import { HeroSection } from "./_components/sections/hero-section";


export default function Page() {
  const { open } = useAppKit();
  const { address, caipAddress } = useAppKitAccount({ namespace:'eip155' })
  const { walletProvider } = useAppKitProvider<Provider>("solana");

  async function onSignMessage() {
    if (!walletProvider || !address) throw Error("user is disconnected");
    const encoded = new TextEncoder().encode("Hello from AppKit");
    const signature = await walletProvider.signMessage(encoded);
    console.log(signature);
  }
  console.log({
    address,
    caipAddress,
  })
  return (
    <>
      <HeroSection />
      <Button onClick={() => open()}>Open Connect Modal</Button>
      <Button onClick={() => open({ namespace: "eip155" })}>EVM</Button>
      <Button onClick={() => open({ namespace: "solana" })}>Solana</Button>
      <Button onClick={() => open({ namespace: "bip122" })}>Bitcoin</Button>
      <Button onClick={() => open({ namespace: "ton" })}>Ton</Button>
      <Button onClick={onSignMessage}>Sign Message</Button>
    </>
  )
}
