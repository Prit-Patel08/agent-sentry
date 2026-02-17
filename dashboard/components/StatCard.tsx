import { LucideIcon } from 'lucide-react';

interface StatCardProps {
    label: string;
    value: string | number;
    icon: LucideIcon;
    trend?: string;
}

export default function StatCard({ label, value, icon: Icon, trend }: StatCardProps) {
    return (
        <div className="rounded-xl border border-gray-800 bg-obsidian-800 p-6 shadow-sm hover:border-accent-500/30 transition-colors">
            <div className="flex items-center justify-between">
                <div>
                    <p className="text-sm font-medium text-gray-400">{label}</p>
                    <p className="mt-2 text-3xl font-bold text-white tracking-tight">{value}</p>
                </div>
                <div className="rounded-lg bg-accent-500/10 p-3 text-accent-500">
                    <Icon size={24} />
                </div>
            </div>
            {trend && (
                <div className="mt-4 flex items-center text-xs text-gray-500">
                    <span className="text-green-400 font-medium mr-1">{trend}</span>
                    <span>vs last session</span>
                </div>
            )}
        </div>
    );
}
