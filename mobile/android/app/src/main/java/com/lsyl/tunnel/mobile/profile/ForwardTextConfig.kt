package com.lsyl.tunnel.mobile.profile

import com.lsyl.tunnel.mobile.tunnel.MAX_MOBILE_FORWARDS
import org.snakeyaml.engine.v2.api.Dump
import org.snakeyaml.engine.v2.api.DumpSettings
import org.snakeyaml.engine.v2.api.Load
import org.snakeyaml.engine.v2.api.LoadSettings
import org.snakeyaml.engine.v2.common.FlowStyle
import org.snakeyaml.engine.v2.exceptions.DuplicateKeyException
import org.snakeyaml.engine.v2.exceptions.YamlEngineException

object ForwardTextConfig {
    private val loadSettings = LoadSettings.builder()
        .setLabel("转发配置")
        .setAllowDuplicateKeys(false)
        .setAllowRecursiveKeys(false)
        .setAllowNonScalarKeys(false)
        .setMaxAliasesForCollections(0)
        .setCodePointLimit(64 * 1024)
        .build()

    private val dumpSettings = DumpSettings.builder()
        .setDefaultFlowStyle(FlowStyle.BLOCK)
        .setIndent(2)
        .setIndicatorIndent(2)
        .setIndentWithIndicator(true)
        .setSplitLines(false)
        .build()

    fun parse(text: String): List<ForwardConfig> {
        val loaded = try {
            Load(loadSettings).loadFromString(text)
        } catch (_: DuplicateKeyException) {
            throw ProfileImportException("配置包含重复字段")
        } catch (err: YamlEngineException) {
            throw ProfileImportException("配置格式错误: ${yamlMessage(err)}")
        }
        val root = loaded as? Map<*, *> ?: throw ProfileImportException("配置顶层必须是映射")
        val rootKeys = stringKeys(root, "配置顶层")
        if (rootKeys != setOf("forwards")) throw ProfileImportException("只允许配置 forwards")
        val rawForwards = root["forwards"] as? List<*>
            ?: throw ProfileImportException("forwards 必须是规则列表")
        val forwards = rawForwards.mapIndexed { index, item -> parseRule(index + 1, item) }
        MobileForwardValidator.validate(forwards)
        return forwards
    }

    fun render(forwards: List<ForwardConfig>): String {
        MobileForwardValidator.validate(forwards)
        val rules = forwards.map { forward ->
            linkedMapOf(
                "name" to forward.name,
                "listen_addr" to forward.listenAddr,
                "server_target" to forward.serverTarget
            )
        }
        return Dump(dumpSettings).dumpToString(linkedMapOf("forwards" to rules))
    }

    private fun parseRule(number: Int, value: Any?): ForwardConfig {
        val rule = value as? Map<*, *> ?: throw ProfileImportException("规则 $number 必须是映射")
        val keys = stringKeys(rule, "规则 $number")
        val allowed = setOf("name", "listen_addr", "server_target")
        val unknown = (keys - allowed).sorted()
        if (unknown.isNotEmpty()) {
            throw ProfileImportException("规则 $number 包含未知字段: ${unknown.joinToString(", ")}")
        }
        allowed.forEach { key ->
            if (key !in keys) throw ProfileImportException("规则 $number 缺少 $key")
        }
        return ForwardConfig(
            name = scalar(rule["name"], "规则 $number 的 name"),
            direction = DIRECTION_CLIENT_TO_SERVER,
            listenAddr = scalar(rule["listen_addr"], "规则 $number 的 listen_addr"),
            serverTarget = scalar(rule["server_target"], "规则 $number 的 server_target")
        )
    }

    private fun stringKeys(map: Map<*, *>, location: String): Set<String> {
        if (map.keys.any { it !is String }) throw ProfileImportException("$location 的字段名必须是文本")
        return map.keys.filterIsInstance<String>().toSet()
    }

    private fun scalar(value: Any?, location: String): String {
        val text = value as? String ?: throw ProfileImportException("$location 必须是文本")
        if (text.isBlank()) throw ProfileImportException("$location 不能为空")
        return text.trim()
    }

    private fun yamlMessage(err: YamlEngineException): String =
        err.message?.lineSequence()?.firstOrNull()?.trim().orEmpty().ifBlank { "无法解析 YAML" }
}

internal object MobileForwardValidator {
    fun validate(forwards: List<ForwardConfig>) {
        if (forwards.isEmpty()) throw ProfileImportException("至少需要一个正向端口")
        if (forwards.size > MAX_MOBILE_FORWARDS) {
            throw ProfileImportException("移动端最多支持 $MAX_MOBILE_FORWARDS 个本地端口")
        }
        val names = mutableSetOf<String>()
        val listenPorts = mutableSetOf<Int>()
        forwards.forEachIndexed { index, forward ->
            val label = forward.name.trim().ifBlank { "第 ${index + 1} 条规则" }
            if (forward.name.isBlank()) throw ProfileImportException("规则 ${index + 1} 的 name 不能为空")
            if (forward.direction != DIRECTION_CLIENT_TO_SERVER) {
                throw ProfileImportException("移动端仅支持正向代理")
            }
            val local = try {
                forward.localEndpoint()
            } catch (_: IllegalArgumentException) {
                throw ProfileImportException("规则 $label 的 listen_addr 格式无效")
            }
            if (local.host != "127.0.0.1") {
                throw ProfileImportException("规则 $label 的本地监听只能使用 127.0.0.1")
            }
            if (local.port < 1024) {
                throw ProfileImportException("规则 $label 的本地端口必须大于等于 1024")
            }
            val target = try {
                HostPort.parse(forward.serverTarget)
            } catch (_: IllegalArgumentException) {
                throw ProfileImportException("规则 $label 的 server_target 格式无效")
            }
            if (!validTargetHost(target.host)) {
                throw ProfileImportException("规则 $label 的 server_target 主机无效")
            }
            if (!names.add(forward.name)) throw ProfileImportException("规则名称重复: ${forward.name}")
            if (!listenPorts.add(local.port)) {
                throw ProfileImportException("本地监听端口重复: ${forward.listenAddr}")
            }
        }
    }

    private fun validTargetHost(host: String): Boolean {
        val candidate = host.removeSuffix(".")
        if (candidate.isEmpty() || candidate.length > 253) return false
        if (candidate.all { it.isDigit() || it == '.' }) return validIpv4(candidate)
        return candidate.split('.').all { DOMAIN_LABEL.matches(it) }
    }

    private fun validIpv4(host: String): Boolean {
        val octets = host.split('.')
        return octets.size == 4 && octets.all { octet ->
            octet.isNotEmpty() &&
                (octet == "0" || !octet.startsWith('0')) &&
                octet.toIntOrNull()?.let { it in 0..255 } == true
        }
    }

    private val DOMAIN_LABEL = Regex("[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?")
}
