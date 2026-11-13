"use client"

import { useState } from "react"
import { Play } from "lucide-react"
import Image from "next/image"

const videoId = "5YONOoyTbQc"

export function SeeItInActionSection() {
  const [isPlaying, setIsPlaying] = useState(false)

  return (
    <section
      className="mx-auto w-[calc(100%-3rem)] py-12 sm:w-[calc(100%-5rem)] sm:py-16"
      aria-labelledby="see-it-in-action-title"
    >
      <h2
        id="see-it-in-action-title"
        className="text-4xl font-medium tracking-tight sm:text-5xl"
      >
        See it in action
      </h2>
      <p className="mt-4 max-w-3xl text-base leading-relaxed text-muted-foreground">
        Follow a transaction from wallet connection to payment release. See how
        to get started with Vecrow.
      </p>

      <div className="mt-8 rounded-2xl border border-border/70 bg-gradient-to-br from-card to-background p-4 sm:mt-10 sm:p-6 lg:p-8">
        <div className="mb-4 flex items-center gap-2 text-sm text-muted-foreground">
          <Play className="size-4 text-primary" />
          Vecrow walkthrough
        </div>
        <div className="relative mx-auto aspect-video w-full max-w-5xl overflow-hidden rounded-xl border border-border bg-background">
          {isPlaying ? (
            <iframe
              className="absolute inset-0 size-full"
              src={`https://www.youtube-nocookie.com/embed/${videoId}?autoplay=1&rel=0`}
              title="Vecrow walkthrough"
              allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share"
              referrerPolicy="strict-origin-when-cross-origin"
              allowFullScreen
            />
          ) : (
            <>
              <Image
                className="object-cover"
                src={`https://img.youtube.com/vi/${videoId}/hqdefault.jpg`}
                alt="Vecrow walkthrough video thumbnail"
                fill
                unoptimized
                sizes="(max-width: 1024px) 100vw, 1024px"
              />
              <div className="absolute inset-0 bg-slate-950/35" />
              <button
                type="button"
                onClick={() => setIsPlaying(true)}
                aria-label="Play Vecrow walkthrough video"
                className="absolute inset-0 grid place-items-center text-slate-950 transition-colors hover:bg-slate-950/10 focus-visible:outline-2 focus-visible:outline-offset-[-6px] focus-visible:outline-primary"
              >
                <span className="grid size-20 place-items-center rounded-full bg-primary-foreground text-primary shadow-lg transition-transform hover:scale-105 sm:size-24">
                  <Play className="ml-1 size-8 fill-current sm:size-9" />
                </span>
              </button>
            </>
          )}
        </div>
        <div className="px-2 pb-2 pt-7 sm:px-2 sm:pt-9">
          <h3 className="text-2xl font-medium tracking-tight sm:text-3xl">
            Your first escrow, step by step.
          </h3>
          <p className="mt-2 text-muted-foreground">
            Connect. Agree. Fund. Deliver. Release.
          </p>
        </div>
      </div>
    </section>
  )
}
