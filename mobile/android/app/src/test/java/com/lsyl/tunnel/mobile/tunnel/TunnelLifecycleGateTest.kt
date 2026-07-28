package com.lsyl.tunnel.mobile.tunnel

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean

class TunnelLifecycleGateTest {
    @Test
    fun stopWaitsForInFlightListenerCreationBeforeClosingResources() {
        val gate = TunnelLifecycleGate()
        val running = AtomicBoolean(true)
        val listenerCreationStarted = CountDownLatch(1)
        val allowListenerCreationToFinish = CountDownLatch(1)
        val stopAttemptStarted = CountDownLatch(1)
        val stopCompleted = CountDownLatch(1)
        val workers = Executors.newFixedThreadPool(2)
        val start = workers.submit {
            gate.runIfRunning(running) {
                listenerCreationStarted.countDown()
                allowListenerCreationToFinish.await(2, TimeUnit.SECONDS)
            }
        }
        assertTrue(listenerCreationStarted.await(1, TimeUnit.SECONDS))
        val stop = workers.submit {
            stopAttemptStarted.countDown()
            gate.stopIfRunning(running) {
                stopCompleted.countDown()
            }
        }
        assertTrue(stopAttemptStarted.await(1, TimeUnit.SECONDS))

        try {
            assertFalse("stop crossed listener creation", stopCompleted.await(200, TimeUnit.MILLISECONDS))
        } finally {
            allowListenerCreationToFinish.countDown()
            start.get(2, TimeUnit.SECONDS)
            stop.get(2, TimeUnit.SECONDS)
            workers.shutdownNow()
        }

        assertFalse(running.get())
        assertTrue(stopCompleted.await(0, TimeUnit.MILLISECONDS))
    }
}
