export interface Incident {
    id: number;
    timestamp: string;
    command: string;
    model_name: string;
    exit_reason: string;
    max_cpu: number;
    pattern: string;
    token_savings_estimate: number;
    reason: string;
    cpu_score: number;
    entropy_score: number;
    confidence_score: number;
    recovery_status: string;
    restart_count: number;
}

export interface TimelineEvent {
    event_id?: string;
    run_id?: string;
    incident_id?: string;
    type: string;
    timestamp: string;
    title: string;
    summary: string;
    reason: string;
    actor?: string;
    pid?: number;
    cpu_score?: number;
    entropy_score?: number;
    confidence_score?: number;
}
