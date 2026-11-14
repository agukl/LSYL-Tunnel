package com.lsyl.tunnel.mobile.profile

import android.content.Context
import java.io.File
import java.util.UUID

private const val LEGACY_ID = "legacy"
private val PROFILE_ID = Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")

data class StoredProfile(
    val id: String,
    val alias: String,
    val serverAddr: String,
    val username: String,
    val active: Boolean,
    val available: Boolean
)

class ProfileStore(context: Context) {
    private val repository = ProfileRepository(context.filesDir)

    fun save(imported: ImportedProfile, alias: String = "") = repository.save(imported, alias)

    fun load(): LoadedProfile? = repository.load()

    fun listProfiles(): List<StoredProfile> = repository.listProfiles()

    fun selectProfile(id: String) = repository.selectProfile(id)

    fun updateForwards(forwards: List<ForwardConfig>) = repository.updateForwards(forwards)

    fun deleteProfile(id: String) = repository.deleteProfile(id)
}

internal class ProfileRepository(private val root: File) {
    private val legacyDir = File(root, "active-profile")
    private val profilesDir = File(root, "profiles")
    private val activeFile = File(root, "active-profile-id")

    fun save(imported: ImportedProfile, alias: String) {
        val id = UUID.randomUUID().toString()
        val files = ProfileFiles(profileDir(id))
        try {
            files.save(imported)
            files.saveAlias(alias)
            setActive(id)
        } catch (err: Exception) {
            files.delete()
            throw err
        }
    }

    fun load(): LoadedProfile? = activeId()?.let { id ->
        runCatching { ProfileFiles(profileDir(id)).load() }.getOrNull()
    }

    fun listProfiles(): List<StoredProfile> {
        val active = activeId()
        return profileIds().map { id ->
            val files = ProfileFiles(profileDir(id))
            val loaded = runCatching { files.load() }.getOrNull()
            val alias = runCatching { files.readAlias() }.getOrDefault("")
            StoredProfile(
                id = id,
                alias = alias.ifBlank { loaded?.profile?.serverAddr ?: "配置不可用" },
                serverAddr = loaded?.profile?.serverAddr.orEmpty(),
                username = loaded?.profile?.username.orEmpty(),
                active = id == active,
                available = loaded != null
            )
        }.sortedWith(compareByDescending<StoredProfile> { it.active }.thenBy { it.alias })
    }

    fun selectProfile(id: String) {
        if (id !in profileIds() || ProfileFiles(profileDir(id)).load() == null) {
            throw ProfileImportException("配置不可用，请重新导入")
        }
        setActive(id)
    }

    fun updateForwards(forwards: List<ForwardConfig>) {
        val id = activeId() ?: throw ProfileImportException("未导入连接配置")
        ProfileFiles(profileDir(id)).updateForwards(forwards)
    }

    fun deleteProfile(id: String) {
        if (id !in profileIds()) throw ProfileImportException("配置不存在")
        val wasActive = id == activeId()
        val dir = profileDir(id)
        ProfileFiles(dir).delete()
        if (dir.exists()) {
            throw ProfileImportException("删除配置失败，请检查存储空间")
        }
        if (wasActive) {
            val next = profileIds().firstOrNull { candidate ->
                runCatching { ProfileFiles(profileDir(candidate)).load() != null }.getOrDefault(false)
            }
            setActive(next)
        }
    }

    private fun profileIds(): List<String> = buildList {
        if (legacyDir.resolve("profile.json").exists() || legacyDir.resolve("server.crt").exists()) add(LEGACY_ID)
        profilesDir.listFiles().orEmpty()
            .filter { it.isDirectory && PROFILE_ID.matches(it.name) }
            .sortedBy { it.name }
            .forEach { add(it.name) }
    }

    private fun activeId(): String? {
        if (activeFile.isFile) return activeFile.readText(Charsets.UTF_8).trim().takeIf { it == LEGACY_ID || PROFILE_ID.matches(it) }
        return LEGACY_ID.takeIf { it in profileIds() }
    }

    private fun profileDir(id: String): File = when {
        id == LEGACY_ID -> legacyDir
        PROFILE_ID.matches(id) -> File(profilesDir, id)
        else -> throw ProfileImportException("配置标识无效")
    }

    private fun setActive(id: String?) {
        if (id != null && id != LEGACY_ID && !PROFILE_ID.matches(id)) throw ProfileImportException("配置标识无效")
        if (!root.exists() && !root.mkdirs()) throw ProfileImportException("无法创建配置目录")
        atomicWriteProfileFile(activeFile, (id ?: "").toByteArray(Charsets.UTF_8))
    }
}

data class LoadedProfile(
    val profile: MobileProfile,
    val serverCertBytes: ByteArray
)
