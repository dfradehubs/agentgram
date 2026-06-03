"use client";

import { TrendingUp, TrendingDown, Minus } from "lucide-react";

interface StatCard {
  label: string;
  value: string | number;
  subValue?: string;
  trend?: "up" | "down" | "neutral";
}

interface StatsCardsProps {
  cards: StatCard[];
}

export function StatsCards({ cards }: StatsCardsProps) {
  return (
    <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
      {cards.map((card) => (
        <div key={card.label} className="rounded-lg border bg-card p-4">
          <p className="text-xs text-muted-foreground">{card.label}</p>
          <div className="mt-1 flex items-center gap-2">
            <p className="text-2xl font-bold">{card.value}</p>
            {card.trend && (
              <span className="text-muted-foreground">
                {card.trend === "up" && <TrendingUp className="h-4 w-4 text-red-500" />}
                {card.trend === "down" && <TrendingDown className="h-4 w-4 text-green-500" />}
                {card.trend === "neutral" && <Minus className="h-4 w-4" />}
              </span>
            )}
          </div>
          {card.subValue && (
            <p className="mt-1 text-xs text-muted-foreground">{card.subValue}</p>
          )}
        </div>
      ))}
    </div>
  );
}
