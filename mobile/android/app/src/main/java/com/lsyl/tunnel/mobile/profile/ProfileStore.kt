package com.lsyl.tunnel.mobile.profile

import android.content.Context
import java.io.File

private const val ACTIVE_DIR = "active-profile"

class ProfileStore(private val context: Context) {
    private val dir: File = File(context.filesDir, ACTIVE_DIR)
    private val files = ProfileFiles(dir)

    fun save(imported: ImportedProfile) {
        files.save(imported)
    }

    fun load(): LoadedProfile? = files.load()

    fun updateForwards(forwards: List<ForwardConfig>) = files.updateForwards(forwards)

    fun delete() {
        files.delete()
    }
}

data class LoadedProfile(
    val profile: MobileProfile,
    val serverCertBytes: ByteArray
)
