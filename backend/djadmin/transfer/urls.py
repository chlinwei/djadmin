from django.urls import path

from .views import TransferDownloadView, TransferUploadCancelView, TransferUploadChunkView, TransferUploadStatusView


urlpatterns = [
    path('transfer/download/', TransferDownloadView.as_view(), name='transfer-download'),
    path('transfer/upload/chunk/', TransferUploadChunkView.as_view(), name='transfer-upload-chunk'),
    path('transfer/upload/status/', TransferUploadStatusView.as_view(), name='transfer-upload-status'),
    path('transfer/upload/cancel/', TransferUploadCancelView.as_view(), name='transfer-upload-cancel'),
]
