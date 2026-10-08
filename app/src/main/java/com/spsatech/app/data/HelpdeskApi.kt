package com.spsatech.app.data

import okhttp3.MultipartBody
import okhttp3.RequestBody
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.HTTP
import retrofit2.http.Multipart
import retrofit2.http.POST
import retrofit2.http.Part
import retrofit2.http.Path

interface HelpdeskApi {
    @POST("api/devices")
    suspend fun registerDevice(@Body request: DeviceRequest)

    @HTTP(method = "DELETE", path = "api/devices", hasBody = true)
    suspend fun unregisterDevice(@Body request: DeviceRequest)

    @POST("api/auth/login")
    suspend fun login(@Body request: LoginRequest): LoginResponse

    @GET("api/me")
    suspend fun me(): User

    @POST("api/me/password")
    suspend fun changePassword(@Body request: ChangePasswordRequest): LoginResponse

    @GET("api/tickets")
    suspend fun tickets(): List<TicketSummary>

    @GET("api/tickets/{id}")
    suspend fun ticket(@Path("id") id: Long): Ticket

    @Multipart
    @POST("api/tickets")
    suspend fun createTicket(
        @Part("title") title: RequestBody,
        @Part("description") description: RequestBody,
        @Part("location") location: RequestBody,
        @Part("department") department: RequestBody,
        @Part photos: List<MultipartBody.Part>,
    ): Ticket

    @Multipart
    @POST("api/tickets/{id}/messages")
    suspend fun addMessage(
        @Path("id") id: Long,
        @Part("body") body: RequestBody,
        @Part photos: List<MultipartBody.Part>,
    ): Ticket
}
