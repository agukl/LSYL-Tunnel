package com.lsyl.tunnel.mobile.service

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class RecoverySignalGateTest {
    @Test
    fun signalArrivingDuringRecoveryIsRunAfterCurrentRecovery() {
        val gate = RecoverySignalGate()
        var recoveries = 0

        assertTrue(gate.tryQueue())
        gate.drain {
            recoveries++
            if (recoveries == 1) assertFalse(gate.tryQueue())
        }

        assertEquals(2, recoveries)
        assertTrue(gate.tryQueue())
    }

    @Test
    fun defaultNetworkIdentityChangeEmitsRecoveryEvenWhenBothNetworksAreAvailable() {
        val tracker = NetworkChangeTracker<String>()

        assertTrue(tracker.update("wifi"))
        assertFalse(tracker.update("wifi"))
        assertTrue(tracker.update("mobile"))
        assertTrue(tracker.update(null))
        assertFalse(tracker.update(null))
    }

    @Test
    fun wifiTrackerPublishesOnlyTransitions() {
        val published = mutableListOf<Boolean?>()
        val tracker = NetworkChangeTracker<Boolean>()

        tracker.publishIfChanged(false, published::add)
        tracker.publishIfChanged(true, published::add)
        tracker.publishIfChanged(true, published::add)
        tracker.publishIfChanged(false, published::add)

        assertEquals(listOf(false, true, false), published)
    }
}
