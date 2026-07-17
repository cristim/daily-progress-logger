@file:OptIn(kotlinx.coroutines.ExperimentalCoroutinesApi::class)

package com.cristim.dailyprogress.core

import android.content.Context
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import java.util.UUID

/**
 * Thin wrapper around the gomobile-generated [mobilecore.Core] handle.
 *
 * Ownership rules:
 * - [dispatcher] is a single-threaded IO dispatcher; ALL Core calls MUST run
 *   on it. This serialises read-modify-write sequences and keeps file I/O off
 *   the main thread (Core methods are synchronous).
 * - [openCore] is safe to call only from within a coroutine running on
 *   [dispatcher]. Callers go through [CoreRepository] which enforces this.
 */
class CoreClient(private val context: Context) {

    /** Single-threaded dispatcher that serialises all Core calls. */
    val dispatcher: CoroutineDispatcher = Dispatchers.IO.limitedParallelism(1)

    /** Lazily opened Core handle. Initialised once on first use. */
    @Volatile
    private var _core: mobilecore.Core? = null

    /**
     * Returns the open [mobilecore.Core] handle, initialising it on first
     * call. Must only be invoked from within a coroutine running on
     * [dispatcher] so file I/O never touches the main thread.
     */
    fun openCore(): mobilecore.Core = _core ?: run {
        val dataDir = context.filesDir.resolve("data").also { it.mkdirs() }
        val clientId = com.cristim.dailyprogress.BuildConfig.GOOGLE_CLIENT_ID
        val deviceId = deviceId(context)
        mobilecore.Mobilecore.open(dataDir.absolutePath, clientId, deviceId)
            .also { _core = it }
    }

    private fun deviceId(ctx: Context): String {
        val prefs = ctx.getSharedPreferences("core_prefs", Context.MODE_PRIVATE)
        return prefs.getString(KEY_DEVICE_ID, null) ?: UUID.randomUUID().toString().also {
            prefs.edit().putString(KEY_DEVICE_ID, it).apply()
        }
    }

    companion object {
        private const val KEY_DEVICE_ID = "device_id"
    }
}
