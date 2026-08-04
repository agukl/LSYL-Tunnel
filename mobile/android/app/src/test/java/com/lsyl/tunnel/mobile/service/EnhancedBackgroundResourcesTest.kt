package com.lsyl.tunnel.mobile.service

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class EnhancedBackgroundResourcesTest {
    private class FakeLock(
        private val acquireFailure: Throwable? = null,
        private val releaseFailure: Throwable? = null
    ) : BackgroundResourceLock {
        override var isHeld = false
            private set
        var acquireCount = 0
            private set
        var releaseCount = 0
            private set

        override fun acquire() {
            acquireFailure?.let { throw it }
            if (isHeld) return
            isHeld = true
            acquireCount++
        }

        override fun release() {
            releaseFailure?.let { throw it }
            if (!isHeld) return
            isHeld = false
            releaseCount++
        }
    }

    @Test
    fun enabledRunningAcquiresCpuAndSupportedWifi() {
        val cpu = FakeLock()
        val wifi = FakeLock()
        val statuses = mutableListOf<EnhancedResourceStatus>()

        EnhancedBackgroundResourceController(cpu, wifi, statuses::add)
            .update(enabled = true, desiredRunning = true, wifiAvailable = true)

        assertTrue(cpu.isHeld)
        assertTrue(wifi.isHeld)
        assertEquals(EnhancedResourceStatus(cpuHeld = true, wifiHeld = true), statuses.last())
    }

    @Test
    fun repeatedUpdateIsIdempotent() {
        val cpu = FakeLock()
        val wifi = FakeLock()
        val controller = EnhancedBackgroundResourceController(cpu, wifi) {}

        controller.update(enabled = true, desiredRunning = true, wifiAvailable = true)
        controller.update(enabled = true, desiredRunning = true, wifiAvailable = true)

        assertEquals(1, cpu.acquireCount)
        assertEquals(1, wifi.acquireCount)
    }

    @Test
    fun mobileNetworkReleasesOnlyWifi() {
        val cpu = FakeLock()
        val wifi = FakeLock()
        val controller = EnhancedBackgroundResourceController(cpu, wifi) {}
        controller.update(enabled = true, desiredRunning = true, wifiAvailable = true)

        controller.update(enabled = true, desiredRunning = true, wifiAvailable = false)

        assertTrue(cpu.isHeld)
        assertFalse(wifi.isHeld)
        assertEquals(1, wifi.releaseCount)
    }

    @Test
    fun disabledOrNotDesiredReleasesEverything() {
        val cpu = FakeLock()
        val wifi = FakeLock()
        val statuses = mutableListOf<EnhancedResourceStatus>()
        val controller = EnhancedBackgroundResourceController(cpu, wifi, statuses::add)
        controller.update(enabled = true, desiredRunning = true, wifiAvailable = true)

        controller.update(enabled = false, desiredRunning = true, wifiAvailable = true)

        assertFalse(cpu.isHeld)
        assertFalse(wifi.isHeld)
        assertEquals(EnhancedResourceStatus(), statuses.last())
    }

    @Test
    fun android14PolicyWithoutWifiLockStillAcquiresCpu() {
        val cpu = FakeLock()
        val statuses = mutableListOf<EnhancedResourceStatus>()

        EnhancedBackgroundResourceController(cpu, null, statuses::add)
            .update(enabled = true, desiredRunning = true, wifiAvailable = true)

        assertTrue(cpu.isHeld)
        assertEquals(EnhancedResourceStatus(cpuHeld = true), statuses.last())
    }

    @Test
    fun partialAcquireFailurePublishesSpecificWarningWithoutThrowing() {
        val statuses = mutableListOf<EnhancedResourceStatus>()

        EnhancedBackgroundResourceController(
            cpuLock = FakeLock(acquireFailure = IllegalStateException("denied")),
            wifiLock = null,
            statusSink = statuses::add
        ).update(enabled = true, desiredRunning = true, wifiAvailable = false)

        assertEquals("CPU 保持失败", statuses.last().warning)
        assertFalse(statuses.last().cpuHeld)
    }

    @Test
    fun releaseFailureIsReportedAndDoesNotPreventOtherRelease() {
        val cpu = FakeLock()
        val wifi = FakeLock(releaseFailure = IllegalStateException("denied"))
        val statuses = mutableListOf<EnhancedResourceStatus>()
        val controller = EnhancedBackgroundResourceController(cpu, wifi, statuses::add)
        controller.update(enabled = true, desiredRunning = true, wifiAvailable = true)

        controller.release()

        assertFalse(cpu.isHeld)
        assertEquals("Wi-Fi 释放失败", statuses.last().warning)
    }
}
