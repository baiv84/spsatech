package com.spsatech.app.ui.newticket

import android.content.Context
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.PhotoCamera
import androidx.compose.material.icons.filled.PhotoLibrary
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.MenuAnchorType
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.spsatech.app.data.HelpdeskRepository
import com.spsatech.app.data.User
import com.spsatech.app.ui.common.PhotoThumb
import com.spsatech.app.ui.common.PhotoViewer
import com.spsatech.app.ui.common.compressPhoto
import com.spsatech.app.ui.common.newCameraUri
import com.spsatech.app.ui.common.repoViewModel
import kotlinx.coroutines.launch

const val MAX_PHOTOS = 5

class NewTicketViewModel(private val repo: HelpdeskRepository, val departments: List<String>) : ViewModel() {
    var title by mutableStateOf("")
    var location by mutableStateOf("")
    /** Если подразделение одно — оно выбрано сразу. */
    var department by mutableStateOf(departments.singleOrNull() ?: "")
    var description by mutableStateOf("")
    val photos = mutableStateListOf<Uri>()
    var sending by mutableStateOf(false)
        private set
    var error by mutableStateOf<String?>(null)
        private set

    fun addPhotos(uris: List<Uri>) {
        photos.addAll(uris.take(MAX_PHOTOS - photos.size))
    }

    fun submit(context: Context, onCreated: (Long) -> Unit) {
        if (sending) return
        error = when {
            title.isBlank() -> "Укажите тему заявки"
            departments.size > 1 && department.isEmpty() -> "Выберите подразделение"
            description.isBlank() -> "Опишите проблему"
            else -> null
        }
        if (error != null) return
        sending = true
        viewModelScope.launch {
            runCatching {
                val bytes = photos.map { compressPhoto(context, it) }
                repo.createTicket(title, description, location, department, bytes)
            }
                .onSuccess { onCreated(it.id) }
                .onFailure { error = it.message ?: "Не удалось отправить заявку" }
            sending = false
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NewTicketScreen(user: User, onBack: () -> Unit, onCreated: (Long) -> Unit) {
    val vm = repoViewModel { NewTicketViewModel(it, user.departments) }
    val context = LocalContext.current
    var cameraUri by rememberSaveable { mutableStateOf<Uri?>(null) }
    var viewing by rememberSaveable { mutableStateOf<Uri?>(null) }

    val takePicture = rememberLauncherForActivityResult(ActivityResultContracts.TakePicture()) { ok ->
        cameraUri?.takeIf { ok }?.let { vm.addPhotos(listOf(it)) }
    }
    val pickPhotos = rememberLauncherForActivityResult(
        ActivityResultContracts.PickMultipleVisualMedia(MAX_PHOTOS)
    ) { vm.addPhotos(it) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Новая заявка") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Назад")
                    }
                },
            )
        },
    ) { padding ->
        Column(
            Modifier
                .fillMaxSize()
                .padding(padding)
                .imePadding()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            val enabled = !vm.sending
            OutlinedTextField(
                value = vm.title,
                onValueChange = { vm.title = it.take(200) },
                label = { Text("Тема *") },
                placeholder = { Text("Например: не включается компьютер") },
                singleLine = true,
                enabled = enabled,
                keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Sentences),
                modifier = Modifier.fillMaxWidth(),
            )
            if (vm.departments.size > 1) {
                DepartmentPicker(vm.departments, vm.department, enabled) { vm.department = it }
            }
            OutlinedTextField(
                value = vm.location,
                onValueChange = { vm.location = it.take(200) },
                label = { Text("Где находится компьютер") },
                placeholder = { Text("Корпус, аудитория, инв. номер") },
                singleLine = true,
                enabled = enabled,
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = vm.description,
                onValueChange = { vm.description = it.take(5000) },
                label = { Text("Описание проблемы *") },
                placeholder = { Text("Что случилось, когда началось, что уже пробовали") },
                minLines = 5,
                enabled = enabled,
                keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Sentences),
                modifier = Modifier.fillMaxWidth(),
            )

            Text(
                "Фото (${vm.photos.size} из $MAX_PHOTOS)",
                style = MaterialTheme.typography.titleSmall,
                modifier = Modifier.padding(top = 4.dp),
            )
            if (vm.photos.isNotEmpty()) {
                LazyRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    itemsIndexed(vm.photos, key = { _, uri -> uri.toString() }) { i, uri ->
                        Box {
                            PhotoThumb(uri, onClick = { viewing = uri })
                            if (enabled) {
                                IconButton(
                                    onClick = { vm.photos.removeAt(i) },
                                    modifier = Modifier
                                        .align(Alignment.TopEnd)
                                        .padding(4.dp)
                                        .size(24.dp)
                                        .background(Color.Black.copy(alpha = 0.6f), CircleShape),
                                ) {
                                    Icon(
                                        Icons.Default.Close, contentDescription = "Убрать фото",
                                        tint = Color.White, modifier = Modifier.size(16.dp),
                                    )
                                }
                            }
                        }
                    }
                }
            }
            if (vm.photos.size < MAX_PHOTOS) {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedButton(
                        enabled = enabled,
                        onClick = {
                            val uri = newCameraUri(context)
                            cameraUri = uri
                            takePicture.launch(uri)
                        },
                    ) {
                        Icon(Icons.Default.PhotoCamera, null, Modifier.size(18.dp))
                        Text("Камера", Modifier.padding(start = 8.dp))
                    }
                    OutlinedButton(
                        enabled = enabled,
                        onClick = {
                            pickPhotos.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly))
                        },
                    ) {
                        Icon(Icons.Default.PhotoLibrary, null, Modifier.size(18.dp))
                        Text("Галерея", Modifier.padding(start = 8.dp))
                    }
                }
            }

            vm.error?.let { Text(it, color = MaterialTheme.colorScheme.error) }

            Button(
                onClick = { vm.submit(context.applicationContext, onCreated) },
                enabled = enabled,
                modifier = Modifier.fillMaxWidth().padding(top = 8.dp).height(52.dp),
            ) {
                if (vm.sending) CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.dp)
                else Text("Отправить заявку")
            }
        }
    }

    viewing?.let { PhotoViewer(it, onDismiss = { viewing = null }) }
}

/** Выбор подразделения для сотрудников, работающих в нескольких. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DepartmentPicker(options: List<String>, selected: String, enabled: Boolean, onSelect: (String) -> Unit) {
    var expanded by remember { mutableStateOf(false) }
    ExposedDropdownMenuBox(expanded = expanded, onExpandedChange = { if (enabled) expanded = it }) {
        OutlinedTextField(
            value = selected,
            onValueChange = {},
            readOnly = true,
            enabled = enabled,
            label = { Text("Подразделение *") },
            placeholder = { Text("По какому подразделению проблема") },
            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded) },
            modifier = Modifier.fillMaxWidth().menuAnchor(MenuAnchorType.PrimaryNotEditable, enabled),
        )
        ExposedDropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            options.forEach {
                DropdownMenuItem(text = { Text(it) }, onClick = { onSelect(it); expanded = false })
            }
        }
    }
}
