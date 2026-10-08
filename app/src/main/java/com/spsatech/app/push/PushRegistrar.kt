package com.spsatech.app.push

import android.util.Log
import com.google.firebase.messaging.FirebaseMessaging
import com.spsatech.app.data.HelpdeskRepository
import com.spsatech.app.data.SessionState
import com.spsatech.app.data.SessionStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine
import com.google.android.gms.tasks.Task
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

/**
 * Привязывает FCM-токен телефона к вошедшему пользователю на сервере:
 * после входа, при смене пользователя и когда Firebase выдаёт новый токен.
 */
class PushRegistrar(
    private val repo: HelpdeskRepository,
    private val session: SessionStore,
    private val scope: CoroutineScope,
) {
    fun start() {
        scope.launch {
            session.state
                .map { (it as? SessionState.LoggedIn)?.session?.let { s -> s.user.id to s.token } }
                .distinctUntilChanged()
                .collect { loggedIn -> if (loggedIn != null) register(null) }
        }
    }

    /** [token] — новый токен из onNewToken; null — спросить текущий у Firebase. */
    fun register(token: String?) {
        scope.launch {
            runCatching {
                val t = token ?: FirebaseMessaging.getInstance().token.await()
                repo.pushToken = t
                if (session.state.value is SessionState.LoggedIn) repo.registerDevice(t)
            }.onFailure { Log.w("Push", "Не удалось зарегистрировать устройство", it) }
        }
    }
}

private suspend fun <T> Task<T>.await(): T = suspendCancellableCoroutine { cont ->
    addOnSuccessListener { cont.resume(it) }
    addOnFailureListener { cont.resumeWithException(it) }
}
