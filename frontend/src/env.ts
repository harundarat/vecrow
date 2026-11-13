import { createEnv } from "@t3-oss/env-nextjs"
import * as z from "zod"

export const env = createEnv({
  server: {
    SECRET: z.string().min(1),
  },
  client: {
    NEXT_PUBLIC_REOWN_PROJECT_ID: z.string().min(1),
  },
  runtimeEnv: {
    SECRET: process.env.SECRET,
    NEXT_PUBLIC_REOWN_PROJECT_ID: process.env.NEXT_PUBLIC_REOWN_PROJECT_ID,
  },
})
