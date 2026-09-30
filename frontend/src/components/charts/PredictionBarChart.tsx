"use client";

import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Cell } from 'recharts';

interface PredictionBarChartProps {
  data: {
    name: string;
    price: number;
    scenario: 'base' | 'bull' | 'bear' | 'issue';
  }[];
}

export default function PredictionBarChart({ data }: PredictionBarChartProps) {
  const getBarColor = (scenario: string) => {
    switch (scenario) {
      case 'bull': return '#10B981'; // emerald
      case 'bear': return '#F43F5E'; // rose
      case 'base': return '#3B82F6'; // blue
      default: return '#94A3B8'; // slate
    }
  };

  return (
    <div className="w-full h-64">
      <ResponsiveContainer width="100%" height="100%">
        <BarChart
          data={data}
          margin={{ top: 20, right: 20, left: 0, bottom: 5 }}
        >
          <CartesianGrid strokeDasharray="3 3" stroke="rgba(51, 65, 85, 0.4)" vertical={false} />
          <XAxis 
            dataKey="name" 
            tick={{ fill: '#94a3b8' }} 
            axisLine={{ stroke: 'rgba(51, 65, 85, 0.8)' }} 
            tickLine={false}
          />
          <YAxis 
            tick={{ fill: '#94a3b8' }} 
            axisLine={{ stroke: 'rgba(51, 65, 85, 0.8)' }} 
            tickLine={false}
            tickFormatter={(value) => `₹${value}`}
          />
          <Tooltip 
            cursor={{ fill: 'rgba(51, 65, 85, 0.2)' }}
            contentStyle={{ 
              backgroundColor: 'rgba(15, 23, 42, 0.9)', 
              borderColor: 'rgba(51, 65, 85, 0.5)',
              borderRadius: '0.75rem',
              color: '#fff',
              boxShadow: '0 10px 15px -3px rgba(0, 0, 0, 0.5)'
            }}
            itemStyle={{ color: '#fff' }}
            formatter={(value: any) => [`₹${value}`, 'Predicted Price']}
          />
          <Bar dataKey="price" radius={[4, 4, 0, 0]}>
            {data.map((entry, index) => (
              <Cell key={`cell-${index}`} fill={getBarColor(entry.scenario)} />
            ))}
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}
