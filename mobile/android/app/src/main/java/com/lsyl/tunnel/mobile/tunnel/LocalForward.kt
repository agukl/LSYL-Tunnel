package com.lsyl.tunnel.mobile.tunnel

import com.lsyl.tunnel.mobile.profile.ForwardConfig
import com.lsyl.tunnel.mobile.protocol.TunnelProtocol
import com.lsyl.tunnel.mobile.protocol.ProtocolException
import com.lsyl.tunnel.mobile.runtime.FailureClassifier
import com.lsyl.tunnel.mobile.runtime.IssueDisposition
import com.lsyl.tunnel.mobile.runtime.RuntimeIssue
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.ServerSocket
import java.net.Socket
import java.net.SocketException
import java.util.concurrent.Executor
import java.util.concurrent.RejectedExecutionException
import java.util.concurrent.atomic.AtomicBoolean

class LocalForward(
    private val forward: ForwardConfig,
    private val protocol: TunnelProtocol,
    private val runtime: ForwardRuntime,
    private val listenerExecutor: Executor,
    private val connectionExecutor: Executor,
    private val copyExecutor: Executor
) {
    private val running = AtomicBoolean(false)
    @Volatile private var serverSocket: ServerSocket? = null

    fun start() {
        if (!running.compareAndSet(false, true)) return
        runtime.setState(ForwardState.STARTING, "正在监听本地端口")
        try {
            val endpoint = forward.localEndpoint()
            val socket = ServerSocket()
            socket.reuseAddress = true
            socket.bind(InetSocketAddress(InetAddress.getByName("127.0.0.1"), endpoint.port))
            serverSocket = socket
            runtime.setState(ForwardState.LISTENING, "本地端口监听中")
            runtime.clearIssue()
            listenerExecutor.execute { acceptLoop(socket) }
        } catch (err: Exception) {
            running.set(false)
            try {
                serverSocket?.close()
            } catch (_: Exception) {
            }
            serverSocket = null
            runtime.setState(ForwardState.LISTEN_FAILED, friendlyMessage(err))
        }
    }

    fun stop(
        message: String = "已停止",
        state: ForwardState = ForwardState.STOPPED,
        issue: RuntimeIssue? = null
    ) {
        running.set(false)
        try {
            serverSocket?.close()
        } catch (_: Exception) {
        } finally {
            serverSocket = null
            runtime.setState(state, message)
            if (issue != null) runtime.reportIssue(issue)
        }
    }

    fun isRunning(): Boolean = running.get()

    private fun acceptLoop(listener: ServerSocket) {
        while (running.get()) {
            try {
                val local = listener.accept()
                try {
                    connectionExecutor.execute { handleLocal(local) }
                } catch (_: RejectedExecutionException) {
                    closeQuietly(local)
                    runtime.reportIssue(ruleIssue("local_connection_limit", "本地连接并发已达上限，已拒绝"))
                }
            } catch (_: SocketException) {
                if (running.get()) {
                    runtime.setState(ForwardState.ERROR, "本地监听已中断")
                    runtime.reportIssue(ruleIssue("local_listener_interrupted", "本地监听已中断"))
                }
                return
            } catch (err: Exception) {
                runtime.setState(ForwardState.ERROR, friendlyMessage(err))
                runtime.reportIssue(ruleIssue("local_listener_error", friendlyMessage(err)))
            }
        }
    }

    private fun handleLocal(local: Socket) {
        val streamDone = runtime.beginStream()
        try {
            val remote = protocol.open(forward)
            runtime.clearIssue()
            SocketPipe.copyBidirectional(local, remote, copyExecutor)
        } catch (err: ProtocolException) {
            closeQuietly(local)
            val issue = classify(err)
            if (issue.disposition == IssueDisposition.RULE_DISABLED) {
                stop(issue.message, ForwardState.REJECTED, issue)
            } else {
                runtime.reportIssue(issue)
            }
        } catch (err: Exception) {
            closeQuietly(local)
            runtime.reportIssue(classify(err))
        } finally {
            streamDone()
        }
    }

    private fun closeQuietly(socket: Socket) {
        try {
            socket.close()
        } catch (_: Exception) {
        }
    }

    private fun friendlyMessage(err: Exception): String = err.message?.takeIf { it.isNotBlank() } ?: "连接异常"

    private fun classify(err: Throwable): RuntimeIssue {
        val endpoint = forward.localEndpoint()
        return FailureClassifier.classify(err, forward.displayName(), endpoint.port)
    }

    private fun ruleIssue(code: String, message: String): RuntimeIssue {
        val endpoint = forward.localEndpoint()
        return RuntimeIssue(code, message, IssueDisposition.CONNECTION_ONLY, forward.displayName(), endpoint.port)
    }
}
