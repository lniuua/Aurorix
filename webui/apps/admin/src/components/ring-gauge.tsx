/**
 * 环形进度图：纯 SVG，无第三方依赖。
 *
 * value 为 0~100；传 undefined 表示数据不可用，只画灰色底环。
 * 颜色随占用升高自动切换：<60% 正常、60~85% 警告、>85% 危险。
 */
export function RingGauge({ value, size = 52 }: { value?: number; size?: number }) {
  const stroke = 6
  const radius = (size - stroke) / 2
  const circumference = 2 * Math.PI * radius
  const percent = value === undefined ? 0 : Math.min(100, Math.max(0, value))
  const offset = circumference * (1 - percent / 100)
  const color =
    percent >= 85 ? 'var(--color-danger)' : percent >= 60 ? 'var(--color-warn)' : 'var(--color-accent-text)'

  return (
    <svg
      width={size}
      height={size}
      viewBox={`0 0 ${size} ${size}`}
      className="shrink-0 -rotate-90"
      role="img"
      aria-label={value === undefined ? '暂无数据' : `${Math.round(percent)}%`}
    >
      <circle
        cx={size / 2}
        cy={size / 2}
        r={radius}
        fill="none"
        stroke="var(--color-subtle)"
        strokeWidth={stroke}
      />
      {value !== undefined && (
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke={color}
          strokeWidth={stroke}
          strokeLinecap="round"
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          style={{ transition: 'stroke-dashoffset 400ms ease, stroke 300ms ease' }}
        />
      )}
    </svg>
  )
}
