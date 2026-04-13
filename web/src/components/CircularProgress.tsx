interface CircularProgressProps {
  value: number
  size?: number
  strokeWidth?: number
  label?: string
  className?: string
  trackClassName?: string
  indicatorClassName?: string
  labelClassName?: string
}

export function CircularProgress({
  value,
  size = 40,
  strokeWidth = 4,
  label,
  className = '',
  trackClassName = 'stroke-border/60',
  indicatorClassName = 'stroke-primary',
  labelClassName = 'fill-foreground text-[10px] font-semibold',
}: CircularProgressProps) {
  const clampedValue = Math.max(0, Math.min(value, 100))
  const radius = (size - strokeWidth) / 2
  const circumference = 2 * Math.PI * radius
  const offset = circumference - (clampedValue / 100) * circumference
  const center = size / 2

  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} className={className}>
      <circle
        cx={center}
        cy={center}
        r={radius}
        fill="none"
        strokeWidth={strokeWidth}
        className={trackClassName}
      />
      <circle
        cx={center}
        cy={center}
        r={radius}
        fill="none"
        strokeWidth={strokeWidth}
        strokeLinecap="round"
        strokeDasharray={circumference}
        strokeDashoffset={offset}
        transform={`rotate(-90 ${center} ${center})`}
        className={indicatorClassName}
      />
      {label && (
        <text x="50%" y="50%" textAnchor="middle" dominantBaseline="middle" className={labelClassName}>
          {label}
        </text>
      )}
    </svg>
  )
}
