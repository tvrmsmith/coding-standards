using System.Collections.Generic;
using System.Threading.Tasks;

namespace Fixtures;

// An async method and an iterator are scored on the lines they are written on, never on the state
// machine the compiler generates from them.
public class Asynchrony
{
    public async Task<int> LoadAsync(int a)
    {
        await Task.Yield();
        if (a > 0)
        {
            return a;
        }

        return 0;
    }

    public IEnumerable<int> Evens(int[] xs)
    {
        foreach (var x in xs)
        {
            if (x % 2 == 0)
            {
                yield return x;
            }
        }
    }

    public async IAsyncEnumerable<int> StreamAsync(int[] xs)
    {
        foreach (var x in xs)
        {
            await Task.Yield();
            yield return x;
        }
    }

    public async Task RunAsync()
    {
        async Task InnerAsync(int n)
        {
            if (n > 0)
            {
                await Task.Yield();
            }
        }

        await InnerAsync(1);
    }
}
