package com.lsyl.tunnel.mobile.service

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class RecoverySignalGateTest {
    @Test
    fun coalescesSignalsUntilQueuedRecoveryCompletes() {
        val gate = RecoverySignalGate()

        assertTrue(gate.tryQueue())
        assertFalse(gate.tryQueue())

        gate.complete()

        assertTrue(gate.tryQueue())
    }
}
