package com.lsyl.tunnel.mobile.profile

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ForwardTargetTest {
    @Test
    fun matchesServerTargetPolicyWithoutDns() {
        listOf("127.0.0.1", "10.0.0.1", "172.16.0.1", "172.31.255.254", "192.168.1.7",
            "fd00::7", "::ffff:192.168.1.7").forEach { assertTrue(it, isForwardTargetHost(it)) }
        listOf("", "localhost", "::1", "127.0.0.2", "::ffff:127.0.0.1", "db.internal",
            "203.0.113.10", "172.15.0.1", "172.32.0.1", "169.254.1.1", "fe80::1",
            "fec0::1", "2001:db8::1", "0.0.0.0", "192.168.01.7", "192.168.1.999",
            "[fd00::7]", "fd00::7%wlan0").forEach { assertFalse(it, isForwardTargetHost(it)) }
    }
}
