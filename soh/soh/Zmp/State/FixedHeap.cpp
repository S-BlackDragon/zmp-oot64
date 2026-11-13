#include "FixedHeap.h"

#ifdef _WIN32
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#endif
#ifdef __EMSCRIPTEN__
#include <string.h>
// ZMP web (D-238): wasm has one linear memory and no fixed virtual addresses, but a static array of the module sits at
// the same address in every browser that runs this wasm, which is all the fixed heap is for. Sized for the game's
// system heap (SYSTEM_HEAP_SIZE, z64.h: 4 MiB); a bigger request falls back to a normal allocation.
#define ZMP_WASM_FIXED_HEAP_SIZE (1024 * 1024 * 4)
alignas(64) static unsigned char sWasmFixedHeap[ZMP_WASM_FIXED_HEAP_SIZE];
#endif

static void* sFixedHeap = nullptr;

extern "C" void* Zmp_AllocFixedHeap(size_t size) {
#ifdef _WIN32
    void* p = VirtualAlloc((void*)(uintptr_t)ZMP_SYSTEM_HEAP_ADDR, size, MEM_RESERVE | MEM_COMMIT, PAGE_READWRITE);
    if (p != (void*)(uintptr_t)ZMP_SYSTEM_HEAP_ADDR) {
        if (p != nullptr) {
            VirtualFree(p, 0, MEM_RELEASE);
        }
        return nullptr;
    }
    // VirtualAlloc returns zeroed pages.
    sFixedHeap = p;
    return p;
#elif defined(__EMSCRIPTEN__)
    if (size > ZMP_WASM_FIXED_HEAP_SIZE || sFixedHeap != nullptr) {
        return nullptr;
    }
    memset(sWasmFixedHeap, 0, sizeof(sWasmFixedHeap));
    sFixedHeap = sWasmFixedHeap;
    return sFixedHeap;
#else
    (void)size;
    return nullptr;
#endif
}

extern "C" int Zmp_FreeFixedHeap(void* heap) {
#ifdef _WIN32
    if (heap != nullptr && heap == sFixedHeap) {
        VirtualFree(heap, 0, MEM_RELEASE);
        sFixedHeap = nullptr;
        return 1;
    }
#elif defined(__EMSCRIPTEN__)
    if (heap != nullptr && heap == sFixedHeap) {
        sFixedHeap = nullptr;
        return 1;
    }
#endif
    (void)heap;
    return 0;
}

extern "C" int Zmp_FixedHeapActive(void) {
    return sFixedHeap != nullptr;
}
