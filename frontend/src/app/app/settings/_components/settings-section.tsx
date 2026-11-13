type SettingsSectionProps = React.PropsWithChildren<{
  id: string
  title: string
  description: React.ReactNode
}>

export function SettingsSection({
  id,
  title,
  description,
  children,
}: SettingsSectionProps) {
  return (
    <section
      id={id}
      aria-labelledby={`${id}-heading`}
      className="flex scroll-mt-24 flex-col gap-4 rounded-2xl bg-card p-4 sm:p-6"
    >
      <div>
        <h2 id={`${id}-heading`} className="text-lg font-medium">
          {title}
        </h2>
        <p className="mt-1 text-sm text-muted-foreground">{description}</p>
      </div>
      {children}
    </section>
  )
}
