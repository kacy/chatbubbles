/// <reference types="vite/client" />

interface BarcodeDetector {
  detect(
    source: ImageBitmapSource,
  ): Promise<Array<{ rawValue?: string }>>;
}

interface BarcodeDetectorConstructor {
  new (options?: { formats?: string[] }): BarcodeDetector;
  getSupportedFormats?: () => Promise<string[]>;
}

interface Window {
  BarcodeDetector?: BarcodeDetectorConstructor;
}
