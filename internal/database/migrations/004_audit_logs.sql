-- Migration: 004_audit_logs.sql
-- Add audit logging table for compliance and security tracking

-- Create audit_logs table
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    event VARCHAR(255) NOT NULL,
    severity VARCHAR(50) NOT NULL,
    user_id VARCHAR(255),
    tenant_id VARCHAR(255),
    resource_id VARCHAR(255),
    resource_type VARCHAR(255),
    action VARCHAR(255) NOT NULL,
    result VARCHAR(255) NOT NULL,
    ip_address INET,
    user_agent TEXT,
    correlation_id VARCHAR(255),
    trace_id VARCHAR(255),
    details JSONB,
    error TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create indexes for audit_logs
CREATE INDEX idx_audit_logs_timestamp ON audit_logs(timestamp);
CREATE INDEX idx_audit_logs_event ON audit_logs(event);
CREATE INDEX idx_audit_logs_severity ON audit_logs(severity);
CREATE INDEX idx_audit_logs_user_id ON audit_logs(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX idx_audit_logs_tenant_id ON audit_logs(tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX idx_audit_logs_resource_id ON audit_logs(resource_id) WHERE resource_id IS NOT NULL;
CREATE INDEX idx_audit_logs_correlation_id ON audit_logs(correlation_id) WHERE correlation_id IS NOT NULL;
CREATE INDEX idx_audit_logs_trace_id ON audit_logs(trace_id) WHERE trace_id IS NOT NULL;
CREATE INDEX idx_audit_logs_ip_address ON audit_logs(ip_address) WHERE ip_address IS NOT NULL;

-- Create composite indexes for common queries
CREATE INDEX idx_audit_logs_user_timestamp ON audit_logs(user_id, timestamp DESC) WHERE user_id IS NOT NULL;
CREATE INDEX idx_audit_logs_tenant_timestamp ON audit_logs(tenant_id, timestamp DESC) WHERE tenant_id IS NOT NULL;
CREATE INDEX idx_audit_logs_event_timestamp ON audit_logs(event, timestamp DESC);
CREATE INDEX idx_audit_logs_severity_timestamp ON audit_logs(severity, timestamp DESC);

-- Create partial indexes for security events
CREATE INDEX idx_audit_logs_security_events ON audit_logs(timestamp DESC) 
WHERE event IN ('access.unauthorized', 'access.permission_denied', 'rate_limit.hit', 'token.revoked');

-- Create GIN index for JSONB details column
CREATE INDEX idx_audit_logs_details ON audit_logs USING GIN(details);

-- Add table comment
COMMENT ON TABLE audit_logs IS 'Audit log table for compliance and security tracking';
COMMENT ON COLUMN audit_logs.event IS 'Type of event being audited (e.g., task.submitted, user.login)';
COMMENT ON COLUMN audit_logs.severity IS 'Severity level: info, warning, error, critical';
COMMENT ON COLUMN audit_logs.user_id IS 'ID of the user who performed the action';
COMMENT ON COLUMN audit_logs.tenant_id IS 'ID of the tenant context';
COMMENT ON COLUMN audit_logs.resource_id IS 'ID of the resource being acted upon';
COMMENT ON COLUMN audit_logs.resource_type IS 'Type of resource (e.g., task, worker)';
COMMENT ON COLUMN audit_logs.action IS 'Action performed (e.g., create, update, delete)';
COMMENT ON COLUMN audit_logs.result IS 'Result of the action (e.g., success, error, blocked)';
COMMENT ON COLUMN audit_logs.ip_address IS 'IP address of the client';
COMMENT ON COLUMN audit_logs.user_agent IS 'User agent string from the request';
COMMENT ON COLUMN audit_logs.correlation_id IS 'Correlation ID for tracing requests';
COMMENT ON COLUMN audit_logs.trace_id IS 'Distributed tracing ID';
COMMENT ON COLUMN audit_logs.details IS 'Additional details about the event in JSON format';
COMMENT ON COLUMN audit_logs.error IS 'Error message if the action failed';

-- Create function to automatically clean up old audit logs
CREATE OR REPLACE FUNCTION cleanup_old_audit_logs(retention_days INTEGER DEFAULT 90)
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM audit_logs 
    WHERE timestamp < NOW() - INTERVAL '1 day' * retention_days;
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Create a view for security-related audit events
CREATE VIEW security_audit_events AS
SELECT 
    id,
    timestamp,
    event,
    severity,
    user_id,
    tenant_id,
    action,
    result,
    ip_address,
    user_agent,
    correlation_id,
    details,
    error
FROM audit_logs
WHERE event IN (
    'access.unauthorized',
    'access.permission_denied', 
    'rate_limit.hit',
    'token.generated',
    'token.revoked',
    'user.login',
    'user.logout'
)
ORDER BY timestamp DESC;

-- Create a view for task-related audit events
CREATE VIEW task_audit_events AS
SELECT 
    id,
    timestamp,
    event,
    severity,
    user_id,
    tenant_id,
    resource_id,
    action,
    result,
    correlation_id,
    details
FROM audit_logs
WHERE event IN (
    'task.submitted',
    'task.completed',
    'task.failed',
    'task.cancelled'
)
ORDER BY timestamp DESC;

-- Grant appropriate permissions (adjust as needed for your setup)
-- GRANT SELECT ON audit_logs TO readonly_user;
-- GRANT SELECT ON security_audit_events TO security_team;
-- GRANT SELECT ON task_audit_events TO operations_team; 