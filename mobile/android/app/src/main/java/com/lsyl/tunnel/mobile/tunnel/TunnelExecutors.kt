package com.lsyl.tunnel.mobile.tunnel

import java.util.concurrent.ArrayBlockingQueue
import java.util.concurrent.Executor
import java.util.concurrent.SynchronousQueue
import java.util.concurrent.ThreadFactory
import java.util.concurrent.ThreadPoolExecutor
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger

internal const val MAX_MOBILE_FORWARDS = 8
private const val MAX_CONNECTION_HANDLERS = 4
private const val MAX_COPY_TASKS = MAX_CONNECTION_HANDLERS * 2
private const val MAX_PENDING_CONNECTIONS = 16

internal class TunnelExecutors {
    val listeners: Executor = newListenerExecutor()
    val connections: Executor = newFixedExecutor(
        name = "lsyl-connection",
        workers = MAX_CONNECTION_HANDLERS,
        pending = MAX_PENDING_CONNECTIONS
    )
    val copies: Executor = newCopyExecutor()

    fun shutdown() {
        listOf(listeners, connections, copies).forEach { executor ->
            (executor as? ThreadPoolExecutor)?.shutdownNow()
        }
    }

    private fun newListenerExecutor(): ThreadPoolExecutor = ThreadPoolExecutor(
        0,
        MAX_MOBILE_FORWARDS,
        30,
        TimeUnit.SECONDS,
        SynchronousQueue<Runnable>(),
        namedThreadFactory("lsyl-listener"),
        ThreadPoolExecutor.AbortPolicy()
    )

    private fun newCopyExecutor(): ThreadPoolExecutor = ThreadPoolExecutor(
        MAX_COPY_TASKS,
        MAX_COPY_TASKS,
        30,
        TimeUnit.SECONDS,
        SynchronousQueue<Runnable>(),
        namedThreadFactory("lsyl-copy"),
        ThreadPoolExecutor.AbortPolicy()
    ).apply {
        allowCoreThreadTimeOut(true)
    }

    private fun newFixedExecutor(name: String, workers: Int, pending: Int): ThreadPoolExecutor = ThreadPoolExecutor(
        workers,
        workers,
        30,
        TimeUnit.SECONDS,
        ArrayBlockingQueue<Runnable>(pending),
        namedThreadFactory(name),
        ThreadPoolExecutor.AbortPolicy()
    ).apply {
        allowCoreThreadTimeOut(true)
    }
}

private fun namedThreadFactory(prefix: String): ThreadFactory {
    val sequence = AtomicInteger(0)
    return ThreadFactory { runnable ->
        Thread(runnable, "$prefix-${sequence.incrementAndGet()}").apply { isDaemon = true }
    }
}
