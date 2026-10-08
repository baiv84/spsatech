package com.spsatech.app.data

import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.MultipartBody
import okhttp3.RequestBody.Companion.toRequestBody
import retrofit2.HttpException
import java.io.IOException
import kotlinx.coroutines.withTimeoutOrNull

/** Ошибка с текстом, который можно показать пользователю. */
class ApiException(message: String) : Exception(message)

class HelpdeskRepository(
    private val api: HelpdeskApi,
    private val session: SessionStore,
    private val json: Json,
) {
    suspend fun login(login: String, password: String): User = call {
        val response = api.login(LoginRequest(login.trim(), password))
        session.save(response.token, response.user)
        response.user
    }

    suspend fun changePassword(current: String, new: String): User = call {
        val response = api.changePassword(ChangePasswordRequest(current, new))
        session.save(response.token, response.user)
        response.user
    }

    /** Выход: сначала отвязываем телефон от push-уведомлений, потом забываем сессию. */
    suspend fun logout() {
        pushToken?.let { token ->
            withTimeoutOrNull(5_000) { runCatching { api.unregisterDevice(DeviceRequest(token)) } }
        }
        session.clear()
    }

    /** Текущий FCM-токен устройства; задаётся из [com.spsatech.app.push.PushRegistrar]. */
    @Volatile
    var pushToken: String? = null

    suspend fun registerDevice(token: String) = call { api.registerDevice(DeviceRequest(token)) }

    /** Обновляет профиль (подразделения, роль могли измениться после синхронизации). */
    suspend fun refreshMe() {
        val token = session.token ?: return
        val user = call { api.me() }
        if (session.token == token) session.save(token, user)
    }

    suspend fun tickets(): List<TicketSummary> = call { api.tickets() }

    suspend fun ticket(id: Long): Ticket = call { api.ticket(id) }

    suspend fun createTicket(
        title: String, description: String, location: String, department: String, photos: List<ByteArray>,
    ): Ticket = call {
        api.createTicket(title.toPart(), description.toPart(), location.toPart(), department.toPart(), photos.toParts())
    }

    suspend fun addMessage(ticketId: Long, body: String, photos: List<ByteArray>): Ticket = call {
        api.addMessage(ticketId, body.toPart(), photos.toParts())
    }

    private fun String.toPart() = trim().toRequestBody("text/plain; charset=utf-8".toMediaType())

    private fun List<ByteArray>.toParts() = mapIndexed { i, bytes ->
        MultipartBody.Part.createFormData(
            "photos", "photo_${i + 1}.jpg", bytes.toRequestBody("image/jpeg".toMediaType())
        )
    }

    /** Переводит сетевые ошибки и ошибки сервера в понятный пользователю текст. */
    private suspend fun <T> call(block: suspend () -> T): T = try {
        block()
    } catch (e: HttpException) {
        val body = e.response()?.errorBody()?.string()
        val parsed = body?.let { runCatching { json.decodeFromString<ApiErrorBody>(it) }.getOrNull() }
        throw ApiException(parsed?.message ?: "Ошибка сервера (${e.code()})")
    } catch (e: IOException) {
        throw ApiException("Нет связи с сервером. Проверьте интернет")
    }
}
