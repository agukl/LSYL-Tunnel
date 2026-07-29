package com.lsyl.tunnel.mobile.service

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ServiceRecoveryPolicyTest {
    @Test
    fun desiredConnectionRecoversWhenServiceIsMissing() {
        assertTrue(ServiceRecoveryPolicy.shouldRecover(desiredRunning = true, serviceActive = false))
    }

    @Test
    fun activeServiceDoesNotReceiveDuplicateRecoveryStart() {
        assertFalse(ServiceRecoveryPolicy.shouldRecover(desiredRunning = true, serviceActive = true))
    }

    @Test
    fun disconnectedIntentDoesNotRestartService() {
        assertFalse(ServiceRecoveryPolicy.shouldRecover(desiredRunning = false, serviceActive = false))
    }

    @Test
    fun processStateTracksServiceLifecycle() {
        val state = ServiceProcessState()

        assertFalse(state.isActive())
        state.markActive()
        assertTrue(state.isActive())
        state.markInactive()
        assertFalse(state.isActive())
    }
}
