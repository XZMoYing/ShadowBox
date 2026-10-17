// Test fixture generator using NekoBox 1.4.2's exact Kryo 5.2.1 dependency.
// java -Dfile.encoding=UTF-8 -cp /tmp/shadowbox-kryo-5.2.1.jar scripts/fixtures/NekoFixture.java
import com.esotericsoftware.kryo.io.ByteBufferOutput;
import java.io.*;
import java.util.*;
import java.util.zip.DeflaterOutputStream;
public class NekoFixture {
 static ByteBufferOutput out;
 static void i(int v){out.writeInt(v);} static void s(String v){out.writeString(v);} static void b(boolean v){out.writeBoolean(v);}
 static void base(int v){i(v);s("example.com");i(443);}
 static void v2(int alter){base(4);s("22222222-2222-4222-8222-222222222222");s(alter==-1?"xtls-rprx-vision":"auto");i(alter);s("ws");s("edge.example.com");s("/socket");i(2048);s("Sec-WebSocket-Protocol");s("tls");s("example.com");s("h2,http/1.1");s("");b(false);s("chrome");s(alter==-1?"base64publickey":"");s("abcd");b(false);s("");i(2);b(false);b(false);i(0);i(1);}
 static void emit(String type,Runnable write)throws Exception{ByteArrayOutputStream raw=new ByteArrayOutputStream();out=new ByteBufferOutput(raw);write.run();i(1);s("测试节点 🍃 "+type);s("");s("");out.close();ByteArrayOutputStream zipped=new ByteArrayOutputStream();try(DeflaterOutputStream z=new DeflaterOutputStream(zipped)){z.write(raw.toByteArray());}System.out.println("sn://"+type+"?"+Base64.getUrlEncoder().withoutPadding().encodeToString(zipped.toByteArray()));}
 public static void main(String[] args)throws Exception{
 emit("ss",()->{base(2);s("aes-128-gcm");s("test password:中文");s("");b(false);});
 emit("socks",()->{base(2);i(2);s("user");s("pw");b(true);});
 emit("vmess",()->v2(0));emit("vmess",()->v2(-1));
 emit("anytls",()->{base(0);s("password");s("example.com");s("h2");s("");s("chrome");b(false);s("");});
 emit("wg",()->{base(2);s("10.0.0.2/32");s("a");s("b");s("");i(1420);s("1,2,3");});
 emit("hysteria",()->{base(7);i(2);i(1);s("hy2 password");i(0);s("secret");s("example.com");s("h3");i(0);i(0);b(false);s("");i(0);i(0);b(false);i(10);s("443,8443-8450");});
 emit("tuic",()->{base(2);s("password");s("");s("native");s("cubic");s("h3");b(false);b(false);i(1500);s("example.com");b(false);b(false);s("");i(5);s("22222222-2222-4222-8222-222222222222");});
 emit("mieru",()->{base(0);s("TCP");s("user");s("password");});
 emit("trojan-go",()->{base(1);s("password");s("example.com");s("ws");s("edge.example.com");s("/socket");s("ss;aes-128-gcm:pw");s("");b(false);});
 }
}
