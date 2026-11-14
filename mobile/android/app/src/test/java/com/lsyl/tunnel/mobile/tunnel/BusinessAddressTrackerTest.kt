package com.lsyl.tunnel.mobile.tunnel

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class BusinessAddressTrackerTest {
    @Test
    fun latestActiveAddressIsShownAndCleared() {
        val changes = mutableListOf<String?>()
        val tracker = BusinessAddressTracker { changes += it }
        val first = tracker.begin("192.0.2.1")
        val second = tracker.begin("2001:db8::1")
        val same = tracker.begin("2001:db8::1")
        assertEquals("2001:db8::1", tracker.current())
        same()
        first()
        assertEquals("2001:db8::1", tracker.current())
        second()
        assertNull(tracker.current())
        assertEquals(listOf("192.0.2.1", "2001:db8::1", null), changes)
    }

    @Test
    fun stoppingClearsAddressAndIgnoresLateStreams() {
        val changes = mutableListOf<String?>()
        val tracker = BusinessAddressTracker { changes += it }
        val done = tracker.begin("192.0.2.1")
        tracker.clear()
        done()
        tracker.begin("2001:db8::1")()
        assertNull(tracker.current())
        assertEquals(listOf("192.0.2.1", null), changes)
    }
}
