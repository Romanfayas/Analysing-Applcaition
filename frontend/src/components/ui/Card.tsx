import { ReactNode } from 'react';

interface CardProps {
  children: ReactNode;
  className?: string;
  gradientHover?: boolean;
  onClick?: () => void;
}

export default function Card({ children, className = '', gradientHover = false, onClick }: CardProps) {
  return (
    <div onClick={onClick} className={`glass-card p-6 ${gradientHover ? 'hover:shadow-[0_0_20px_rgba(59,130,246,0.15)] hover:border-blue-500/30' : ''} ${className}`}>
      {children}
    </div>
  );
}

export function CardHeader({ title, subtitle, action }: { title: string, subtitle?: string, action?: ReactNode }) {
  return (
    <div className="flex justify-between items-start mb-6">
      <div>
        <h3 className="text-xl font-heading font-semibold text-white">{title}</h3>
        {subtitle && <p className="text-sm text-slate-400 mt-1">{subtitle}</p>}
      </div>
      {action && <div>{action}</div>}
    </div>
  );
}
