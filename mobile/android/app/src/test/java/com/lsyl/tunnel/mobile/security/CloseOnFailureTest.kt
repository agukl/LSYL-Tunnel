package com.lsyl.tunnel.mobile.security

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertSame
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.Closeable
import java.io.IOException

class CloseOnFailureTest {
    @Test
    fun failedSocketSetupClosesOwnedResourceImmediately() {
        val resource = RecordingCloseable()
        val failure = IOException("connect failed")

        val thrown = assertThrows(IOException::class.java) {
            resource.closeOnFailure { throw failure }
        }

        assertSame(failure, thrown)
        assertTrue(resource.closed)
    }

    @Test
    fun successfulSocketSetupTransfersOwnershipWithoutClosing() {
        val resource = RecordingCloseable()

        val result = resource.closeOnFailure { "connected" }

        assertEquals("connected", result)
        assertFalse(resource.closed)
    }

    @Test
    fun pendingResourceRegistryClosesOnlyResourcesWhoseOwnershipWasNotReleased() {
        val pending = RecordingCloseable()
        val released = RecordingCloseable()
        val registry = PendingCloseables<RecordingCloseable>()
        registry.track(pending)
        registry.track(released)
        registry.release(released)

        registry.cancelAll()
        val trackedAfterCancellation = RecordingCloseable()
        registry.track(trackedAfterCancellation)

        assertTrue(pending.closed)
        assertFalse(released.closed)
        assertTrue(trackedAfterCancellation.closed)
    }

    private class RecordingCloseable : Closeable {
        var closed = false

        override fun close() {
            closed = true
        }
    }
}
