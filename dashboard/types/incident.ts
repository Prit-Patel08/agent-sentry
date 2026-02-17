export interface Incident {
    id: number;
    timestamp: string;
    command: string;
    model_name: string;
    exit_reason: string;
    max_cpu: number;
    pattern: string;
    token_savings_estimate: number;
}
