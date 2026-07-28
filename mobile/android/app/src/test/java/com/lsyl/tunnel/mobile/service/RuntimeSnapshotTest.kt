package com.lsyl.tunnel.mobile.service

import com.lsyl.tunnel.mobile.runtime.IssueDisposition
import com.lsyl.tunnel.mobile.runtime.RuntimeIssue
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

class RuntimeSnapshotTest {
    @Test
    fun snapshotRoundTripKeepsOnlySafeRuleDetails() {
        val source = RuntimeSnapshot(
            phase = TunnelPhase.DEGRADED,
            summary = "部分端口不可用",
            listenerCount = 1,
            issues = listOf(
                RuntimeIssue(
                    code = "target_denied",
                    message = "当前账号没有访问该端口的权限",
                    disposition = IssueDisposition.RULE_DISABLED,
                    ruleName = "rdp",
                    localPort = 13389
                )
            )
        )

        val encoded = source.toJson()

        assertFalse(encoded.contains("server_target"))
        assertFalse(encoded.contains("192.168."))
        assertEquals(source, RuntimeSnapshot.fromJson(encoded))
    }
}
