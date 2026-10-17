// Original vector geometry, generated locally; no upstream branding.
import AppKit
import Foundation
let root = URL(fileURLWithPath: FileManager.default.currentDirectoryPath)
let output = root.appendingPathComponent("apps/desktop/macos/Runner/Assets.xcassets/AppIcon.appiconset")
func color(_ r: CGFloat,_ g: CGFloat,_ b: CGFloat)->NSColor {NSColor(srgbRed:r/255,green:g/255,blue:b/255,alpha:1)}
for size in [16,32,48,64,128,256,512,1024] {
 let bitmap=NSBitmapImageRep(bitmapDataPlanes:nil,pixelsWide:size,pixelsHigh:size,bitsPerSample:8,samplesPerPixel:4,hasAlpha:true,isPlanar:false,colorSpaceName:.deviceRGB,bytesPerRow:0,bitsPerPixel:0)!
 NSGraphicsContext.saveGraphicsState()
 NSGraphicsContext.current=NSGraphicsContext(bitmapImageRep:bitmap)
 let ctx=NSGraphicsContext.current!.cgContext; ctx.scaleBy(x:CGFloat(size)/1024,y:CGFloat(size)/1024)
 color(40,108,83).setFill(); NSBezierPath(roundedRect:NSRect(x:64,y:64,width:896,height:896),xRadius:220,yRadius:220).fill()
 color(229,243,223).setStroke();let ring=NSBezierPath(roundedRect:NSRect(x:250,y:250,width:524,height:524),xRadius:142,yRadius:142);ring.lineWidth=70;ring.stroke()
 color(181,214,186).setStroke();for y in [390,512,634] {let p=NSBezierPath();p.move(to:NSPoint(x:390,y:y));p.line(to:NSPoint(x:634,y:y));p.lineWidth=42;p.lineCapStyle = .round;p.stroke()}
 color(40,108,83).setStroke();let cut=NSBezierPath();cut.move(to:NSPoint(x:776,y:782));cut.line(to:NSPoint(x:242,y:248));cut.lineWidth=90;cut.stroke()
 NSGraphicsContext.restoreGraphicsState()
 let data=bitmap.representation(using:.png,properties:[:])!;try data.write(to:output.appendingPathComponent("app_icon_\(size).png"))
 if size==256 {try data.write(to:root.appendingPathComponent("assets/shadowbox.png"))}
}
