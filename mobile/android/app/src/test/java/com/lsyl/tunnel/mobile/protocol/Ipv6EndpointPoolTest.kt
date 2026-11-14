package com.lsyl.tunnel.mobile.protocol

import com.lsyl.tunnel.mobile.profile.HostPort
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.net.SocketTimeoutException

class Ipv6EndpointPoolTest {
    @Test
    fun loginResponseReadsOptionalIpv6Candidates() {
        val old = OpenResponse.fromJson(JSONObject("""{"ok":true,"code":"ok"}"""))
        assertTrue(old.businessEntries.isEmpty())

        val current = OpenResponse.fromJson(
            JSONObject("""{"ok":true,"business_entries":["[2001:db8::1]:3443","[2001:db8::2]:3443"]}""")
        )
        assertEquals(listOf("[2001:db8::1]:3443", "[2001:db8::2]:3443"), current.businessEntries)
    }

    @Test
    fun filtersInvalidCandidatesAndPrefersIpv6() {
        val pool = Ipv6EndpointPool(
            listOf("[2001:db8::1]:3443", "[2001:db8::1]:3443", "[::1]:3443", "[fc00::1]:3443",
                "[2001:db8::2]:443", "192.0.2.1:3443", "[2001:db8::2]:3443"),
            HostPort("example.test", 3443)
        )
        val dialed = mutableListOf<String>()
        val selected = pool.dial(5000) { endpoint, timeout ->
            dialed += endpoint.host
            assertEquals(1200, timeout)
            endpoint.host
        }

        assertEquals("2001:db8::1", selected)
        assertEquals(listOf("2001:db8::1"), dialed)
    }

    @Test
    fun tcpFailuresFallBackToOriginalPortAndObserveCooldown() {
        var now = 1000L
        val pool = Ipv6EndpointPool(
            listOf("[2001:db8::1]:3443", "[2001:db8::2]:3443", "[2001:db8::3]:3443"),
            HostPort("example.test", 3443),
            nowMillis = { now }
        )
        val dialed = mutableListOf<Pair<String, Int>>()
        fun attempt(): String = pool.dial(5000) { endpoint, timeout ->
            dialed += endpoint.host to timeout
            if (endpoint.host != "example.test") throw SocketTimeoutException("candidate timed out")
            "bootstrap"
        }

        assertEquals("bootstrap", attempt())
        assertEquals(3, dialed.size)
        assertEquals("example.test" to 5000, dialed.last())
        assertEquals(2, dialed.count { it.second == 1200 })
        assertEquals("bootstrap", attempt())
        assertEquals("2001:db8::3" to 1200, dialed[3])
        assertEquals("example.test" to 5000, dialed[4])
        assertTrue(pool.hasCandidates)
        now += 60_001
        assertTrue(pool.dial(5000) { endpoint, _ -> endpoint.host }.startsWith("2001:db8::"))
    }

    @Test
    fun emptyPoolUsesOnlyOriginalEndpoint() {
        val pool = Ipv6EndpointPool(listOf("[::1]:3443"), HostPort("example.test", 3443))
        assertFalse(pool.hasCandidates)
        assertEquals("example.test", pool.dial(5000) { endpoint, _ -> endpoint.host })
    }
}
