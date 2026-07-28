package com.lsyl.tunnel.mobile.profile

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test

class ForwardTextConfigTest {
    @Test
    fun parsesRestrictedForwardYaml() {
        val parsed = ForwardTextConfig.parse(
            """
            forwards:
              - name: rdp
                listen_addr: 127.0.0.1:13389
                server_target: 192.168.1.7:3389
              - name: database
                listen_addr: 127.0.0.1:13306
                server_target: db.internal:3306
            """.trimIndent()
        )

        assertEquals(2, parsed.size)
        assertEquals("192.168.1.7:3389", parsed[0].serverTarget)
        assertEquals("db.internal:3306", parsed[1].serverTarget)
        assertTrue(parsed.all { it.direction == DIRECTION_CLIENT_TO_SERVER })
    }

    @Test
    fun renderedTextRoundTripsWithoutExposingDirection() {
        val source = listOf(
            ForwardConfig("rdp office", DIRECTION_CLIENT_TO_SERVER, "127.0.0.1:13389", "192.168.1.7:3389")
        )

        val rendered = ForwardTextConfig.render(source)

        assertFalse(rendered.contains("direction"))
        assertEquals(source, ForwardTextConfig.parse(rendered))
    }

    @Test
    fun rejectsUnknownFieldsAndWrongHierarchy() {
        val cases = listOf(
            InvalidCase("forwards: []\nserver_addr: example.test:3443", "只允许配置 forwards"),
            InvalidCase(
                """
                forwards:
                  - name: rdp
                    listen_addr: 127.0.0.1:13389
                    server_target: 192.168.1.7:3389
                    direction: server_to_client
                """.trimIndent(),
                "规则 1 包含未知字段: direction"
            ),
            InvalidCase("forwards:\n  name: rdp", "forwards 必须是规则列表"),
            InvalidCase("forwards:\n  - name: rdp", "规则 1 缺少 listen_addr")
        )

        cases.forEach { assertRejected(it.yaml, it.message) }
    }

    @Test
    fun rejectsDuplicateYamlKeys() {
        assertRejected(
            """
            forwards:
              - name: rdp
                name: duplicate
                listen_addr: 127.0.0.1:13389
                server_target: 192.168.1.7:3389
            """.trimIndent(),
            "配置包含重复字段"
        )
    }

    @Test
    fun rejectsUnsafeOrDuplicateMobileRules() {
        val cases = listOf(
            InvalidCase(ruleYaml("rdp", "0.0.0.0:13389", "192.168.1.7:3389"), "规则 rdp 的本地监听只能使用 127.0.0.1"),
            InvalidCase(ruleYaml("rdp", "127.0.0.1:22", "192.168.1.7:3389"), "规则 rdp 的本地端口必须大于等于 1024"),
            InvalidCase(ruleYaml("rdp", "127.0.0.1:13389", "missing-port"), "规则 rdp 的 server_target 格式无效"),
            InvalidCase(ruleYaml("rdp", "127.0.0.1:13389", "https://host:443"), "规则 rdp 的 server_target 主机无效"),
            InvalidCase(ruleYaml("rdp", "127.0.0.1:13389", "bad host:443"), "规则 rdp 的 server_target 主机无效"),
            InvalidCase(ruleYaml("rdp", "127.0.0.1:13389", "\"[::1]:443\""), "规则 rdp 的 server_target 主机无效"),
            InvalidCase(ruleYaml("rdp", "127.0.0.1:13389", "999.1.1.1:443"), "规则 rdp 的 server_target 主机无效"),
            InvalidCase(
                """
                forwards:
                  - name: rdp
                    listen_addr: 127.0.0.1:13389
                    server_target: 192.168.1.7:3389
                  - name: rdp
                    listen_addr: 127.0.0.1:13390
                    server_target: 192.168.1.8:3389
                """.trimIndent(),
                "规则名称重复: rdp"
            ),
            InvalidCase(
                """
                forwards:
                  - name: rdp-a
                    listen_addr: 127.0.0.1:13389
                    server_target: 192.168.1.7:3389
                  - name: rdp-b
                    listen_addr: 127.0.0.1:13389
                    server_target: 192.168.1.8:3389
                """.trimIndent(),
                "本地监听端口重复: 127.0.0.1:13389"
            ),
            InvalidCase(
                """
                forwards:
                  - name: rdp-a
                    listen_addr: 127.0.0.1:13389
                    server_target: 192.168.1.7:3389
                  - name: rdp-b
                    listen_addr: 127.0.0.1:013389
                    server_target: 192.168.1.8:3389
                """.trimIndent(),
                "本地监听端口重复: 127.0.0.1:013389"
            )
        )

        cases.forEach { assertRejected(it.yaml, it.message) }
    }

    @Test
    fun rejectsEmptyOrExcessiveRuleLists() {
        assertRejected("forwards: []", "至少需要一个正向端口")
        val rules = (1..9).joinToString("\n") { index ->
            "  - name: rule-$index\n    listen_addr: 127.0.0.1:${14000 + index}\n    server_target: 192.168.1.7:${3300 + index}"
        }
        assertRejected("forwards:\n$rules", "移动端最多支持 8 个本地端口")
    }

    private fun assertRejected(yaml: String, expectedMessage: String) {
        try {
            ForwardTextConfig.parse(yaml)
            fail("expected config to be rejected: $yaml")
        } catch (err: ProfileImportException) {
            assertTrue("unexpected message: ${err.message}", err.message?.contains(expectedMessage) == true)
        }
    }

    private fun ruleYaml(name: String, listen: String, target: String): String =
        """
        forwards:
          - name: $name
            listen_addr: $listen
            server_target: $target
        """.trimIndent()

    private data class InvalidCase(val yaml: String, val message: String)
}
