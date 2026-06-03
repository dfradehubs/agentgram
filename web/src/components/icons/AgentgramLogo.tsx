interface AgentgramLogoProps {
  className?: string;
}

export function AgentgramLogo({ className = "h-5 w-5" }: AgentgramLogoProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
    >
      {/* Paper plane body (Telegram style) */}
      <path
        d="M2.5 12.5L21.5 3.5L17 21L12 14.5L2.5 12.5Z"
        fill="currentColor"
        opacity="0.9"
      />
      <path
        d="M12 14.5L17 21L21.5 3.5L12 14.5Z"
        fill="currentColor"
        opacity="0.7"
      />
      {/* Fold line */}
      <path
        d="M12 14.5L9.5 19.5L2.5 12.5L21.5 3.5L12 14.5Z"
        stroke="currentColor"
        strokeWidth="0.3"
        fill="none"
        opacity="0.3"
      />
      {/* Robot antenna - small circle on top */}
      <circle
        cx="14"
        cy="2.5"
        r="1.2"
        fill="currentColor"
        opacity="0.85"
      />
      {/* Antenna stick */}
      <line
        x1="14"
        y1="3.7"
        x2="16"
        y2="6.5"
        stroke="currentColor"
        strokeWidth="1"
        strokeLinecap="round"
        opacity="0.85"
      />
    </svg>
  );
}
