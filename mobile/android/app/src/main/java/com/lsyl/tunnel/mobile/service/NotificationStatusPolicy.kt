package com.lsyl.tunnel.mobile.service

enum class InactiveNotificationDisposition {
    HIDE,
    SHOW_FAILURE
}

object NotificationStatusPolicy {
    fun inactiveDisposition(snapshot: RuntimeSnapshot): InactiveNotificationDisposition =
        if (snapshot.phase == TunnelPhase.FAILED) {
            InactiveNotificationDisposition.SHOW_FAILURE
        } else {
            InactiveNotificationDisposition.HIDE
        }

    fun refreshEntry(previous: RuntimeSnapshot): RuntimeSnapshot =
        if (previous.phase == TunnelPhase.STARTING) {
            previous
        } else {
            RuntimeSnapshot(
                phase = TunnelPhase.STARTING,
                summary = "正在检查连接",
                listenerCount = previous.listenerCount
            )
        }
}
