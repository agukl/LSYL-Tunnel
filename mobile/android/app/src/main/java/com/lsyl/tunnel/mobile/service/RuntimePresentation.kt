package com.lsyl.tunnel.mobile.service

data class RuntimePresentation(
    val status: String,
    val details: String,
    val canConnect: Boolean,
    val canDisconnect: Boolean,
    val canRefresh: Boolean,
    val canEditConfig: Boolean,
    val canReplaceProfile: Boolean
)

object RuntimePresenter {
    fun present(
        snapshot: RuntimeSnapshot,
        hasProfile: Boolean,
        desiredRunning: Boolean
    ): RuntimePresentation {
        val active = desiredRunning && snapshot.phase in ACTIVE_PHASES
        val idle = !desiredRunning && snapshot.phase in IDLE_PHASES
        return RuntimePresentation(
            status = snapshot.summary,
            details = snapshot.issues.distinctBy { Triple(it.code, it.ruleName, it.localPort) }
                .joinToString("\n") { issue ->
                    when {
                        issue.ruleName != null && issue.localPort != null && issue.message.contains(issue.localPort.toString()) ->
                            "${issue.ruleName}：${issue.message}"
                        issue.ruleName != null && issue.localPort != null ->
                            "${issue.ruleName}（本地端口 ${issue.localPort}）：${issue.message}"
                        issue.ruleName != null -> "${issue.ruleName}：${issue.message}"
                        else -> issue.message
                    }
                },
            canConnect = hasProfile && idle,
            canDisconnect = active,
            canRefresh = desiredRunning && snapshot.phase in REFRESHABLE_PHASES,
            canEditConfig = hasProfile && idle,
            canReplaceProfile = !active
        )
    }

    private val ACTIVE_PHASES = setOf(
        TunnelPhase.STARTING,
        TunnelPhase.CONNECTED,
        TunnelPhase.DEGRADED,
        TunnelPhase.WAITING_NETWORK,
        TunnelPhase.STOPPING
    )
    private val REFRESHABLE_PHASES = setOf(
        TunnelPhase.CONNECTED,
        TunnelPhase.DEGRADED,
        TunnelPhase.WAITING_NETWORK
    )
    private val IDLE_PHASES = setOf(TunnelPhase.DISCONNECTED, TunnelPhase.FAILED)
}
